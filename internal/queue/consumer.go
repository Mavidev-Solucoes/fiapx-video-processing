package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/identity"
	amqp "github.com/rabbitmq/amqp091-go"
)

type JobHandler func(context.Context, string) (bool, error)

// Consume reconnects after broker/network interruptions. Deliveries are
// manually acknowledged only when the job is durably in a terminal state.
// A transient error or duplicate while a lease is busy is requeued.
func (r *Rabbit) Consume(ctx context.Context, handle JobHandler) error {
	for ctx.Err() == nil {
		err := r.consumeOnce(ctx, handle)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			log.Printf("RabbitMQ consumer reconnecting: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(3 * time.Second):
		}
	}
	return nil
}

func (r *Rabbit) consumeOnce(ctx context.Context, handle JobHandler) error {
	conn, err := amqp.DialConfig(r.url, amqp.Config{
		Dial: amqp.DefaultDial(5 * time.Second),
	})
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if _, err := ch.QueueDeclare(JobsQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.Qos(1, 0, false); err != nil {
		return err
	}
	deliveries, err := ch.Consume(JobsQueue, "", false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("consumer delivery channel closed")
			}
			var message struct {
				JobID string `json:"jobId"`
			}
			if err := json.Unmarshal(delivery.Body, &message); err != nil || !identity.ValidUUID(message.JobID) {
				log.Printf("rejecting invalid queue message")
				if err := delivery.Reject(false); err != nil {
					return err
				}
				continue
			}
			acknowledge, handleErr := handle(ctx, message.JobID)
			if handleErr != nil {
				log.Printf("job %s will be redelivered: %v", message.JobID, handleErr)
			}
			if acknowledge {
				if err := delivery.Ack(false); err != nil {
					return err
				}
				continue
			}
			// Avoid an immediate hot loop while the job is leased or a
			// dependency is unavailable. The database lease will expire
			// after a crashed worker releases its RabbitMQ delivery.
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(3 * time.Second):
			}
			if err := delivery.Nack(false, true); err != nil {
				return err
			}
		}
	}
}
