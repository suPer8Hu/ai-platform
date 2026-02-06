package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/suPer8Hu/ai-platform/internal/ai"
	"github.com/suPer8Hu/ai-platform/internal/chat"
	"github.com/suPer8Hu/ai-platform/internal/config"
	"github.com/suPer8Hu/ai-platform/internal/db"
)

const (
	retryHeaderKey  = "x-retry-count"
	errorHeaderKey  = "x-last-error"
	maxRetryDefault = 5

	// test-only switches (no effect unless you set env vars)
	testFailJobEnv     = "FAIL_JOB_ID"      // always fail for this job_id (drives into DLQ)
	testFailJobOnceEnv = "FAIL_ONCE_JOB_ID" // fail only once for this job_id (validates retry then success)
)

type jobMsg struct {
	JobID string `json:"job_id"`
}

func workerConcurrency() int {
	v := os.Getenv("WORKER_CONCURRENCY")
	if v == "" {
		return 2
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 2
	}
	if n > 50 {
		return 50
	}
	return n
}

func maxRetries() int {
	v := strings.TrimSpace(os.Getenv("WORKER_MAX_RETRIES"))
	if v == "" {
		return maxRetryDefault
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return maxRetryDefault
	}
	if n > 20 {
		return 20
	}
	return n
}

// exponential backoff with cap, in milliseconds
func retryDelayMs(retryCount int) int32 {
	// retryCount is the *next* attempt number (1..)
	// 1: 1s, 2: 2s, 3: 4s, 4: 8s, 5: 16s ... cap at 60s
	d := time.Second * time.Duration(1<<max(0, retryCount-1))
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return int32(d / time.Millisecond)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func getRetryCount(d amqp.Delivery) int {
	if d.Headers == nil {
		return 0
	}
	v, ok := d.Headers[retryHeaderKey]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int32:
		return int(t)
	case int64:
		return int(t)
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(t)
		return n
	case []byte:
		n, _ := strconv.Atoi(string(t))
		return n
	default:
		return 0
	}
}

// publishToQueue republishes the same payload with headers to a queue.
func publishToQueue(ctx context.Context, ch *amqp.Channel, queue string, body []byte, headers amqp.Table) error {
	pub := amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
		Headers:     headers,
		// persistent message
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
	}
	return ch.PublishWithContext(ctx, "", queue, false, false, pub)
}

func shouldFailJob(jobID string) bool {
	failID := strings.TrimSpace(os.Getenv(testFailJobEnv))
	return failID != "" && failID == jobID
}

var failOnceSeen sync.Map

func shouldFailJobOnce(jobID string) bool {
	failID := strings.TrimSpace(os.Getenv(testFailJobOnceEnv))
	if failID == "" || failID != jobID {
		return false
	}
	_, loaded := failOnceSeen.LoadOrStore(jobID, true)
	return !loaded
}

func main() {
	cfg := config.Load()

	gdb := db.Connect(cfg.DBDSN)

	repo := chat.NewRepo(gdb)

	// Provider
	// var provider ai.Provider
	// switch cfg.AIProvider {
	// case "", "ollama":
	// 	provider = ai.NewOllamaProvider(cfg.OllamaBaseURL, cfg.OllamaModel)
	// default:
	// 	log.Fatalf("unsupported AI_PROVIDER=%q", cfg.AIProvider)
	// }

	// Provider registry (route by session.Provider + session.Model)
	reg := ai.NewRegistry()

	// Register Ollama (default)
	reg.Register("ollama", func(ctx context.Context, model string) (ai.Provider, error) {
		_ = ctx
		m := strings.TrimSpace(model)
		if m == "" {
			m = cfg.OllamaModel
		}
		return ai.NewOllamaProvider(cfg.OllamaBaseURL, m), nil
	})

	// Register OpenRouter (OpenAI-compatible)
	reg.Register("openrouter", func(ctx context.Context, model string) (ai.Provider, error) {
		_ = ctx
		m := strings.TrimSpace(model)
		if m == "" {
			m = cfg.OpenRouterModel
		}
		return ai.NewOpenRouterProvider(
			cfg.OpenRouterBaseURL,
			cfg.OpenRouterAPIKey,
			m,
			cfg.OpenRouterSiteURL,
			cfg.OpenRouterAppName,
		), nil
	})

	svc := chat.NewService(repo, reg, cfg.ChatContextWindowSize)

	conn, err := amqp.Dial(cfg.RabbitURL)
	if err != nil {
		log.Fatalf("rabbit dial: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("rabbit channel: %v", err)
	}
	defer ch.Close()

	// Queue names
	mainQ := cfg.RabbitQueue
	retryQ := cfg.RabbitQueue + ".retry"
	dlqQ := cfg.RabbitQueue + ".dlq"

	// Declare DLQ first
	_, err = ch.QueueDeclare(dlqQ, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("queue declare dlq: %v", err)
	}

	// Retry queue: TTL + dead-letter back to main queue
	_, err = ch.QueueDeclare(retryQ, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": mainQ,
		// NOTE: x-message-ttl is per-queue; we will override per-message via "expiration"
	})
	if err != nil {
		log.Fatalf("queue declare retry: %v", err)
	}

	// Main queue: dead-letter to DLQ when rejected/nacked(requeue=false) or expired
	_, err = ch.QueueDeclare(mainQ, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": dlqQ,
	})
	if err != nil {
		log.Fatalf("queue declare main: %v", err)
	}

	//  strict concurrency control
	concurrency := workerConcurrency()
	maxR := maxRetries()

	if err := ch.Qos(concurrency, 0, false); err != nil {
		log.Fatalf("qos: %v", err)
	}

	msgs, err := ch.Consume(mainQ, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("worker started, queue=%s concurrency=%d max_retries=%d", mainQ, concurrency, maxR)

	// worker pool
	jobs := make(chan amqp.Delivery, concurrency*2)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			defer wg.Done()
			for d := range jobs {
				var m jobMsg
				if err := json.Unmarshal(d.Body, &m); err != nil || m.JobID == "" {
					log.Printf("worker=%d bad message: %v", workerID, err)
					// reject -> goes to DLQ by main queue's DLX
					_ = d.Reject(false)
					continue
				}

				start := time.Now()

				// test-only failure injection, fail should before processing
				var err error
				if shouldFailJobOnce(m.JobID) {
					// disable after first trigger so the retry attempt can proceed
					err = fmt.Errorf("simulated failure once (FAIL_ONCE_JOB_ID=%s)", m.JobID)
				} else if shouldFailJob(m.JobID) {
					err = fmt.Errorf("simulated failure (FAIL_JOB_ID=%s)", m.JobID)
				} else {
					err = handleJob(ctx, svc, repo, m.JobID)
				}

				if err != nil {
					cost := time.Since(start)
					retryCount := getRetryCount(d)
					nextRetry := retryCount + 1

					log.Printf("worker=%d job=%s failed cost=%s retry=%d err=%v", workerID, m.JobID, cost, retryCount, err)

					// Decide retry vs DLQ
					if retryCount < maxR {
						// Publish to retry queue with incremented retry count and delay.
						h := amqp.Table{}
						for k, v := range d.Headers {
							h[k] = v
						}
						h[retryHeaderKey] = int32(nextRetry)
						h[errorHeaderKey] = truncateErr(err)

						delay := retryDelayMs(nextRetry)
						pub := amqp.Publishing{
							ContentType:  "application/json",
							Body:         d.Body,
							Headers:      h,
							DeliveryMode: amqp.Persistent,
							Timestamp:    time.Now(),
							Expiration:   strconv.Itoa(int(delay)), // per-message TTL in ms
						}

						if pubErr := ch.PublishWithContext(ctx, "", retryQ, false, false, pub); pubErr != nil {
							// If we can't re-publish, do NOT ack; let main-queue DLQ handle it via reject.
							log.Printf("worker=%d republish-retry failed job=%s err=%v", workerID, m.JobID, pubErr)
							_ = d.Reject(false)
							continue
						}

						// Ack original so it doesn't stay unacked / redeliver immediately.
						if ackErr := d.Ack(false); ackErr != nil {
							log.Printf("worker=%d ack-after-republish failed job=%s err=%v", workerID, m.JobID, ackErr)
						}
						continue
					}

					// Exceeded retries: send to DLQ with context, then ack.
					h := amqp.Table{}
					for k, v := range d.Headers {
						h[k] = v
					}
					h[retryHeaderKey] = int32(retryCount)
					h[errorHeaderKey] = truncateErr(err)

					if pubErr := publishToQueue(ctx, ch, dlqQ, d.Body, h); pubErr != nil {
						log.Printf("worker=%d publish-dlq failed job=%s err=%v", workerID, m.JobID, pubErr)
						// fallback: reject to main queue's DLQ routing
						_ = d.Reject(false)
						continue
					}

					if ackErr := d.Ack(false); ackErr != nil {
						log.Printf("worker=%d ack-after-dlq failed job=%s err=%v", workerID, m.JobID, ackErr)
					}
					continue
				}

				if err := d.Ack(false); err != nil {
					log.Printf("worker=%d ack failed job=%s err=%v", workerID, m.JobID, err)
				}
			}
		}(i)
	}

	// dispatcher
	for {
		select {
		case <-ctx.Done():
			log.Printf("worker shutting down")
			close(jobs)
			wg.Wait()
			return

		case d, ok := <-msgs:
			if !ok {
				log.Printf("delivery channel closed")
				time.Sleep(1 * time.Second)
				continue
			}
			jobs <- d
		}
	}
}

func handleJob(ctx context.Context, svc *chat.Service, repo *chat.Repo, jobID string) error {
	jobStart := time.Now()

	t0 := time.Now()
	_ = repo.UpdateJobStatusRunning(ctx, jobID)
	updateCost := time.Since(t0)

	t1 := time.Now()
	j, err := repo.GetJobByID(ctx, jobID)
	getJobCost := time.Since(t1)
	if err != nil {
		if time.Since(jobStart) > 500*time.Millisecond {
			log.Printf("job_timing job=%s update=%s getJob=%s total=%s err=%v",
				jobID, updateCost, getJobCost, time.Since(jobStart), err,
			)
		}
		return err
	}

	t2 := time.Now()
	reply, assistantMsgID, err := svc.GenerateAssistantReplyAndInsert(ctx, j.UserID, j.SessionID)
	genCost := time.Since(t2)

	if err != nil {
		t3 := time.Now()
		_ = repo.MarkJobFailed(ctx, jobID, err.Error())
		markFailCost := time.Since(t3)

		log.Printf("job_timing_failed job=%s update=%s getJob=%s gen=%s markFail=%s total=%s err=%v",
			jobID, updateCost, getJobCost, genCost, markFailCost, time.Since(jobStart), err,
		)
		return err
	}
	_ = reply

	t4 := time.Now()
	if err := repo.MarkJobSucceeded(ctx, jobID, assistantMsgID); err != nil {
		markSuccCost := time.Since(t4)
		log.Printf("job_timing_failed job=%s update=%s getJob=%s gen=%s markSucc=%s total=%s err=%v",
			jobID, updateCost, getJobCost, genCost, markSuccCost, time.Since(jobStart), err,
		)
		return err
	}
	markSuccCost := time.Since(t4)

	total := time.Since(jobStart)

	if total > 2*time.Second {
		log.Printf("job_timing job=%s update=%s getJob=%s gen=%s markSucc=%s total=%s",
			jobID, updateCost, getJobCost, genCost, markSuccCost, total,
		)
	}

	return nil
}

// truncateErr keeps headers small
func truncateErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
