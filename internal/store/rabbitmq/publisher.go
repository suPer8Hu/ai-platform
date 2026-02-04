package rabbitmq

import (
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Publisher struct {
	conn  *amqp.Connection
	ch    *amqp.Channel
	queue string
}

type JobMessage struct {
	JobID string `json:"job_id"`
}

func NewPublisher(url, queue string) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	// match worker
	mainQ := queue
	retryQ := queue + ".retry"
	dlqQ := queue + ".dlq"

	// DLQ
	if _, err := ch.QueueDeclare(
		dlqQ,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false,
		nil,
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	// Retry queue: message TTL -> dead-letter back to main queue
	if _, err := ch.QueueDeclare(
		retryQ,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": mainQ,
		},
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	// Main queue: dead-letter to DLQ on reject/nack(requeue=false)
	if _, err := ch.QueueDeclare(
		mainQ,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    "",
			"x-dead-letter-routing-key": dlqQ,
		},
	); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, err
	}

	return &Publisher{conn: conn, ch: ch, queue: queue}, nil
}

func (p *Publisher) Close() error {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

func (p *Publisher) PublishJob(ctx context.Context, jobID string) error {
	body, err := json.Marshal(JobMessage{JobID: jobID})
	if err != nil {
		return err
	}

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return p.ch.PublishWithContext(cctx,
		"",      // default exchange
		p.queue, // routing key = queue
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
			Timestamp:    time.Now(),
		},
	)
}
