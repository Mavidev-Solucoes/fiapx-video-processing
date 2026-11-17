package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	client *minio.Client
	bucket string
}

func NewFromEnv() (*Client, error) {
	raw := os.Getenv("S3_ENDPOINT")
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return nil, errors.New("S3_ENDPOINT must be an http(s) URL")
	}
	access := os.Getenv("S3_ACCESS_KEY")
	secret := os.Getenv("S3_SECRET_KEY")
	bucket := os.Getenv("S3_BUCKET")
	if access == "" || secret == "" || bucket == "" {
		return nil, errors.New("S3_ACCESS_KEY, S3_SECRET_KEY and S3_BUCKET are required")
	}
	client, err := minio.New(endpoint.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(access, secret, ""),
		Secure: endpoint.Scheme == "https",
	})
	if err != nil {
		return nil, fmt.Errorf("configure object storage: %w", err)
	}
	return &Client{client: client, bucket: bucket}, nil
}

func (s *Client) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if _, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return fmt.Errorf("store object: %w", err)
	}
	return nil
}

func (s *Client) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("open stored object: %w", err)
	}
	if _, err := object.Stat(); err != nil {
		_ = object.Close()
		return nil, fmt.Errorf("stat stored object: %w", err)
	}
	return object, nil
}

func (s *Client) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}
