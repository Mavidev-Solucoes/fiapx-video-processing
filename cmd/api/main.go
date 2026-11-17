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
	"github.com/Mavidev-Solucoes/fiapx-video-processing/internal/httpapi"
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

	server := &http.Server{
		Addr:              addr,
		Handler:           httpapi.New(database, database, tokens),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown failed: %v", err)
		}
	}()

	log.Printf("FIAP X API listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
