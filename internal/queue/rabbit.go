package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const JobsQueue = "fiapx.jobs"

type Rabbit struct {
	url string
}

func New(url string) (*Rabbit, error) {
	if url == "" {
		return nil, errors.New("RABBITMQ_URL is required")
	}
	return &Rabbit{url: url}, nil
}

func (r *Rabbit) PublishJob(ctx context.Context, jobID string) error {
	conn, err := amqp.DialConfig(r.url, amqp.Config{
		Dial: amqp.DefaultDial(5 * time.Second),
	})
	if err != nil {
		return fmt.Errorf("connect to RabbitMQ: %w", err)
	}
	defer conn.Close()
	channel, err := conn.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()
	if _, err := channel.QueueDeclare(JobsQueue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := channel.Confirm(false); err != nil {
		return err
	}
	confirms := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	body, err := json.Marshal(map[string]string{"jobId": jobID})
	if err != nil {
		return err
	}
	if err := channel.PublishWithContext(ctx, "", JobsQueue, true, false, amqp.Publishing{
		ContentType: "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId: jobID,
		Body: body,
	}); err != nil {
		return err
	}
	select {
	case returned := <-returns:
		return fmt.Errorf("RabbitMQ returned message: %d %s", returned.ReplyCode, returned.ReplyText)
	case confirm, ok := <-confirms:
		if !ok || !confirm.Ack {
			return errors.New("RabbitMQ did not acknowledge the job message")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
