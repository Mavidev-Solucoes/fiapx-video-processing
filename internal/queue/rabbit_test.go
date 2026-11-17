package queue

import (
	"context"
	"errors"
	"testing"
)

func TestRabbitConfigAndUnavailableBroker(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("empty RabbitMQ URL accepted")
	}
	rabbit, err := New("amqp://guest:guest@127.0.0.1:1/")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rabbit.Consume(ctx, func(context.Context, string) (bool, error) {
		t.Fatal("handler called on canceled context")
		return false, nil
	}); err != nil {
		t.Fatalf("canceled consumer: %v", err)
	}
	if err := rabbit.PublishJob(context.Background(), "e95dc09c-6b41-49d1-8934-91a9cdf27819"); err == nil {
		t.Fatal("publishing succeeded without a broker")
	}
	if err := rabbit.consumeOnce(context.Background(), nil); err == nil {
		t.Fatal("consumer connected without a broker")
	}
}

func TestPublishCancellationDoesNotHang(t *testing.T) {
	rabbit, err := New("amqp://guest:guest@127.0.0.1:1/")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rabbit.PublishJob(ctx, "e95dc09c-6b41-49d1-8934-91a9cdf27819"); err == nil ||
		errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected broker or canceled connection error: %v", err)
	}
}
