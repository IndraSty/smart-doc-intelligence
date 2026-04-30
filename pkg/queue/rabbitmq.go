package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/IndraSty/smart-doc-intelligence/config"
	"github.com/IndraSty/smart-doc-intelligence/internal/domain"
	"github.com/IndraSty/smart-doc-intelligence/pkg/logger"
)

// Client wraps a RabbitMQ connection and channel for publishing and consuming
// document processing jobs.
type Client struct {
	conn    *amqp.Connection
	channel *amqp.Channel
	cfg     *config.RabbitMQConfig
	log     *logger.Logger
}

// NewClient creates a RabbitMQ client, opens a channel, and declares the queue.
// The queue is durable so messages survive broker restarts.
func NewClient(cfg *config.RabbitMQConfig, log *logger.Logger) (*Client, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("queue.NewClient dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("queue.NewClient open channel: %w", err)
	}

	// Declare durable queue — survives broker restart
	_, err = ch.QueueDeclare(
		cfg.QueueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("queue.NewClient declare queue: %w", err)
	}

	// Prefetch limits how many unacknowledged messages a worker holds at once
	// This prevents one slow worker from starving others
	if err := ch.Qos(cfg.PrefetchCount, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("queue.NewClient set QoS: %w", err)
	}

	log.Info().
		Str("queue", cfg.QueueName).
		Int("prefetch", cfg.PrefetchCount).
		Msg("RabbitMQ connection established")

	return &Client{
		conn:    conn,
		channel: ch,
		cfg:     cfg,
		log:     log,
	}, nil
}

// Publish serializes a QueueMessage and publishes it to the queue.
// Messages are persistent so they survive broker restarts.
func (c *Client) Publish(ctx context.Context, msg *domain.QueueMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("queue.Publish marshal: %w", err)
	}

	err = c.channel.PublishWithContext(ctx,
		"",              // default exchange
		c.cfg.QueueName, // routing key = queue name
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // survive broker restart
			Timestamp:    time.Now(),
			Body:         data,
		},
	)
	if err != nil {
		return fmt.Errorf("queue.Publish: %w", err)
	}

	c.log.Debug().
		Str("job_id", msg.JobID).
		Str("document_id", msg.DocumentID).
		Str("queue", c.cfg.QueueName).
		Msg("Message published to queue")

	return nil
}

// Consume returns a channel of deliveries for the worker pool to process.
// Each delivery must be explicitly Ack'd or Nack'd by the consumer.
func (c *Client) Consume(consumerTag string) (<-chan amqp.Delivery, error) {
	deliveries, err := c.channel.Consume(
		c.cfg.QueueName,
		consumerTag,
		false, // autoAck = false — we manually ack after successful processing
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return nil, fmt.Errorf("queue.Consume: %w", err)
	}

	c.log.Info().
		Str("consumer", consumerTag).
		Str("queue", c.cfg.QueueName).
		Msg("Consumer registered")

	return deliveries, nil
}

// ParseMessage deserializes a RabbitMQ delivery body into a QueueMessage.
func ParseMessage(delivery amqp.Delivery) (*domain.QueueMessage, error) {
	msg := &domain.QueueMessage{}
	if err := json.Unmarshal(delivery.Body, msg); err != nil {
		return nil, fmt.Errorf("queue.ParseMessage unmarshal: %w", err)
	}
	return msg, nil
}

// HealthCheck verifies the RabbitMQ connection is alive.
func (c *Client) HealthCheck() error {
	if c.conn == nil || c.conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection is closed")
	}
	return nil
}

// Close gracefully closes the channel and connection.
// Call this on application shutdown.
func (c *Client) Close() {
	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			c.log.Error().Err(err).Msg("Failed to close RabbitMQ channel")
		}
	}

	if c.conn != nil && !c.conn.IsClosed() {
		if err := c.conn.Close(); err != nil {
			c.log.Error().Err(err).Msg("Failed to close RabbitMQ connection")
		}
	}

	c.log.Info().Msg("RabbitMQ connection closed")
}
