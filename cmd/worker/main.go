package main

import (
	"context"
	"encoding/json"
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

	_, err = ch.QueueDeclare(cfg.RabbitQueue, true, false, false, false, nil)
	if err != nil {
		log.Fatalf("queue declare: %v", err)
	}

	//  strict concurrency control
	concurrency := workerConcurrency()

	if err := ch.Qos(concurrency, 0, false); err != nil {
		log.Fatalf("qos: %v", err)
	}

	msgs, err := ch.Consume(cfg.RabbitQueue, "", false, false, false, false, nil)
	if err != nil {
		log.Fatalf("consume: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("worker started, queue=%s concurrency=%d", cfg.RabbitQueue, concurrency)

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
					_ = d.Nack(false, false)
					continue
				}

				start := time.Now()
				if err := handleJob(ctx, svc, repo, m.JobID); err != nil {
					log.Printf("worker=%d job %s failed cost=%s err=%v", workerID, m.JobID, time.Since(start), err)
					_ = d.Nack(false, false)
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
