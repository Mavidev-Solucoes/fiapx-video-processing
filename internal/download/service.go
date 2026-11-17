package download

import (
	"context"
	"io"
)

type JobOutputs interface {
	GetCompletedOutput(context.Context, string, string) (string, error)
}

type Objects interface {
	Get(context.Context, string) (io.ReadCloser, error)
}

type Service struct {
	jobs    JobOutputs
	objects Objects
}

func New(jobs JobOutputs, objects Objects) *Service {
	return &Service{jobs: jobs, objects: objects}
}

// An object is accessed only after PostgreSQL confirms ownership and completion.
func (s *Service) OpenForUser(ctx context.Context, userID, jobID string) (io.ReadCloser, error) {
	key, err := s.jobs.GetCompletedOutput(ctx, userID, jobID)
	if err != nil {
		return nil, err
	}
	return s.objects.Get(ctx, key)
}
