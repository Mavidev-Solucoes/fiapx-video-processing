package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/queue"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/storage"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/store"
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("worker database: %v", err)
	}
	defer database.DB.Close()
	objects, err := storage.NewFromEnv()
	if err != nil {
		log.Fatalf("worker storage: %v", err)
	}
	broker, err := queue.New(os.Getenv("RABBITMQ_URL"))
	if err != nil {
		log.Fatalf("worker queue: %v", err)
	}
	runner := worker.New(database, objects)
	log.Println("FIAP X worker started; awaiting RabbitMQ jobs")
	if err := broker.Consume(ctx, runner.Handle); err != nil {
		log.Fatalf("worker consumer: %v", err)
	}
}
