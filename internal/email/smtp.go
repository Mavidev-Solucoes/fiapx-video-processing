package email

import "context"

type Sender struct{}

func NewFromEnv() (*Sender, error) {
	return nil, nil
}

func (s *Sender) SendFailure(context.Context, string, string, int64) error {
	return nil
}
