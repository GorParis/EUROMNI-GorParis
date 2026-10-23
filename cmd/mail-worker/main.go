package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"euromni/internal/mailer"
	"euromni/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	database, err := store.Open(dsn)
	if err != nil {
		return fmt.Errorf("connect to DATABASE_URL: %w", err)
	}
	defer func() {
		if err := database.SQL.Close(); err != nil {
			log.Printf("close mail worker database: %v", err)
		}
	}()
	config, err := mailer.ConfigFromEnv()
	if err != nil {
		return fmt.Errorf("configure mail delivery: %w", err)
	}
	sender, err := mailer.NewSMTPSender(config)
	if err != nil {
		return fmt.Errorf("configure SMTP sender: %w", err)
	}
	worker, err := mailer.NewWorker(database, sender, config.PollInterval, log.Default())
	if err != nil {
		return fmt.Errorf("configure mail worker: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("euromni mail worker polling outbox; SMTP TLS mode=%s", config.TLSMode)
	if err := worker.Run(ctx); err != nil {
		return fmt.Errorf("run mail worker: %w", err)
	}
	return nil
}
