package storage

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestObjectStoreConfiguration(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "")
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("S3_SECRET_KEY", "")
	t.Setenv("S3_BUCKET", "")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("empty storage configuration accepted")
	}
	t.Setenv("S3_ENDPOINT", "ftp://localhost:9000")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("unsupported object URL scheme accepted")
	}
	t.Setenv("S3_ENDPOINT", "http://127.0.0.1:1")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("missing credentials accepted")
	}
	t.Setenv("S3_ACCESS_KEY", "test-key")
	t.Setenv("S3_SECRET_KEY", "test-secret")
	t.Setenv("S3_BUCKET", "fiapx-videos")
	client, err := NewFromEnv()
	if err != nil || client == nil {
		t.Fatalf("valid storage client config: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.Put(ctx, "key", bytes.NewReader([]byte("hello")), 5, "text/plain"); err == nil {
		t.Fatal("object upload succeeded without storage service")
	}
	if stream, err := client.Get(ctx, "missing"); err == nil {
		_ = stream.Close()
		t.Fatal("missing object unexpectedly found")
	}
	if err := client.Delete(ctx, "missing"); err == nil {
		t.Fatal("object deletion succeeded without storage service")
	}
}
