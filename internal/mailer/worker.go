package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"euromni/internal/store"
)

const (
	defaultMaxAttempts = 8
	messageLease       = 45 * time.Second
)

type Worker struct {
	queue        store.EmailOutboxQueue
	sender       Sender
	pollInterval time.Duration
	maxAttempts  int
	logger       *log.Logger
}

func NewWorker(queue store.EmailOutboxQueue, sender Sender, pollInterval time.Duration, logger *log.Logger) (*Worker, error) {
	if queue == nil || sender == nil || pollInterval < 250*time.Millisecond || pollInterval > time.Minute {
		return nil, errors.New("invalid mail worker configuration")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Worker{
		queue: queue, sender: sender, pollInterval: pollInterval,
		maxAttempts: defaultMaxAttempts, logger: logger,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	if w == nil || ctx == nil {
		return errors.New("mail worker is unavailable")
	}
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		claimed, err := w.DeliverOne(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Printf("mail delivery attempt failed: %v", err)
		}
		if claimed {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) DeliverOne(ctx context.Context) (bool, error) {
	if w == nil || w.queue == nil || w.sender == nil {
		return false, errors.New("mail worker is unavailable")
	}
	message, err := w.queue.ClaimNextEmail(ctx, messageLease)
	if err != nil || message == nil {
		return false, err
	}
	var email Message
	switch message.EventType {
	case "order_confirmation":
		var payload store.OrderConfirmationPayload
		if err := decodeEmailPayload(message.Payload, &payload); err != nil {
			return true, w.failPermanently(ctx, message, fmt.Errorf("decode order confirmation payload: %w", err))
		}
		email, err = RenderOrderConfirmation(payload, message.Recipient, message.IdempotencyKey)
	case "order_status":
		var payload store.OrderStatusPayload
		if err := decodeEmailPayload(message.Payload, &payload); err != nil {
			return true, w.failPermanently(ctx, message, fmt.Errorf("decode order status payload: %w", err))
		}
		email, err = RenderOrderStatus(payload, message.Recipient, message.IdempotencyKey)
	default:
		return true, w.failPermanently(ctx, message, fmt.Errorf("unsupported mail event type %q", message.EventType))
	}
	if err != nil {
		return true, w.failPermanently(ctx, message, err)
	}
	if err := w.sender.Send(ctx, email); err != nil {
		if message.Attempts >= w.maxAttempts {
			if failErr := w.queue.RetryEmail(ctx, message.ID, time.Now(), err.Error(), true); failErr != nil {
				return true, fmt.Errorf("mark exhausted email delivery failed: %w", failErr)
			}
			return true, fmt.Errorf("email delivery exhausted after %d attempts: %w", message.Attempts, err)
		}
		nextAttempt := time.Now().Add(retryDelay(message.Attempts))
		if failErr := w.queue.RetryEmail(ctx, message.ID, nextAttempt, err.Error(), false); failErr != nil {
			return true, fmt.Errorf("schedule email delivery retry: %w", failErr)
		}
		return true, fmt.Errorf("email delivery attempt %d: %w", message.Attempts, err)
	}
	if err := w.queue.MarkEmailSent(ctx, message.ID); err != nil {
		return true, fmt.Errorf("mark delivered email as sent: %w", err)
	}
	return true, nil
}

func decodeEmailPayload(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("email payload contains trailing JSON")
	}
	return nil
}

func (w *Worker) failPermanently(ctx context.Context, message *store.EmailOutboxMessage, cause error) error {
	if err := w.queue.RetryEmail(ctx, message.ID, time.Now(), cause.Error(), true); err != nil {
		return fmt.Errorf("mark invalid email event failed: %w", err)
	}
	return cause
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second
	for index := 1; index < attempt && delay < time.Hour; index++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
