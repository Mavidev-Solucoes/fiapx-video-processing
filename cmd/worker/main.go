package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// This process is intentionally a scaffold. RabbitMQ consumption, retry handling,
// status updates and processing orchestration will be added in a dedicated PR.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Print("FIAP X worker foundation started; message consumption is NOT implemented yet")
	<-ctx.Done()
	log.Print("FIAP X worker stopped")
}
