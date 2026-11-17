package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/auth"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/download"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/httpapi"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/intake"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/monitoring"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/queue"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/storage"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
)

func main() {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	tokens, err := auth.NewTokens(os.Getenv("AUTH_JWT_SECRET"))
	if err != nil {
		log.Fatalf("authentication configuration: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("database configuration: %v", err)
	}
	defer database.DB.Close()

	objects, err := storage.NewFromEnv()
	if err != nil {
		log.Fatalf("storage configuration: %v", err)
	}
	publisher, err := queue.New(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatalf("broker configuration: %v", err)
	}
	go dispatchPending(ctx, database, publisher)

	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9091"
	}
	metricsServer := &http.Server{
		Addr:              metricsAddr,
		Handler:           monitoring.Handler(monitoring.Postgres{DB: database.DB}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("FIAP X metrics listening on %s (internal network only)", metricsAddr)
		if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("metrics server: %v", err)
		}
	}()

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.NewWithNotifications(database, database, tokens, intake.New(objects, database), download.New(database, objects), database),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown failed: %v", err)
		}
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("metrics shutdown failed: %v", err)
		}
	}()

	log.Printf("FIAP X API listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func dispatchPending(ctx context.Context, database *store.Postgres, publisher *queue.Rabbit) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		count, err := database.PublishPending(batchCtx, publisher)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("outbox retry scheduled: %v", err)
		} else if count > 0 {
			log.Printf("published %d queued video jobs", count)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
