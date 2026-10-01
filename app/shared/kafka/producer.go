package kafka

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"gin/app/shared/configs"
)

// Publisher sends domain events to a message bus.
type Publisher interface {
	Publish(ctx context.Context, topic string, event any) error
	Enabled() bool
	Close() error
}

type noopPublisher struct{}

func (noopPublisher) Publish(context.Context, string, any) error { return nil }
func (noopPublisher) Enabled() bool                              { return false }
func (noopPublisher) Close() error                               { return nil }

type kafkaPublisher struct {
	writer *kafka.Writer
	prefix string
}

// New creates a publisher. When Kafka is disabled a no-op publisher is
// returned so application code never has to branch on configuration.
func New(cfg configs.KafkaConfig) Publisher {
	if !cfg.Enabled {
		return noopPublisher{}
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Balancer:     &kafka.Hash{},
		BatchTimeout: 10 * time.Millisecond,
		RequiredAcks: kafka.RequireOne,
	}
	return &kafkaPublisher{writer: w, prefix: cfg.TopicPrefix}
}

func (p *kafkaPublisher) Enabled() bool { return true }

func (p *kafkaPublisher) topic(name string) string {
	if p.prefix == "" {
		return name
	}
	return strings.TrimSuffix(p.prefix, ".") + "." + name
}

func (p *kafkaPublisher) Publish(ctx context.Context, topic string, event any) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: p.topic(topic),
		Key:   []byte(topic),
		Value: body,
		Time:  time.Now().UTC(),
	})
}

func (p *kafkaPublisher) Close() error { return p.writer.Close() }

// PublishSafe logs failures instead of propagating them: event bus problems
// must never break an API response.
func PublishSafe(ctx context.Context, p Publisher, topic string, event any) {
	if p == nil || !p.Enabled() {
		return
	}
	if err := p.Publish(ctx, topic, event); err != nil {
		log.Printf("kafka: publish %s failed: %v", topic, err)
	}
}
