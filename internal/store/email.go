package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrEmailOutboxUnavailable = errors.New("email outbox unavailable")

type OrderConfirmationPayload struct {
	OrderNumber   string                  `json:"order_number"`
	CustomerName  string                  `json:"customer_name"`
	Locale        string                  `json:"locale"`
	Currency      string                  `json:"currency"`
	Subtotal      string                  `json:"subtotal"`
	ShippingTotal string                  `json:"shipping_total"`
	GrandTotal    string                  `json:"grand_total"`
	Items         []OrderConfirmationItem `json:"items"`
}

type OrderConfirmationItem struct {
	Name      string `json:"name"`
	Quantity  int    `json:"quantity"`
	LineTotal string `json:"line_total"`
}

type EmailOutboxMessage struct {
	ID             string
	ShopID         string
	OrderID        string
	EventType      string
	IdempotencyKey string
	Recipient      string
	Payload        []byte
	Attempts       int
}

type EmailOutboxQueue interface {
	ClaimNextEmail(ctx context.Context, lease time.Duration) (*EmailOutboxMessage, error)
	MarkEmailSent(ctx context.Context, id string) error
	RetryEmail(ctx context.Context, id string, availableAt time.Time, reason string, permanently bool) error
}

func (s *Store) ClaimNextEmail(ctx context.Context, lease time.Duration) (*EmailOutboxMessage, error) {
	if s == nil || s.SQL == nil || lease <= 0 {
		return nil, ErrEmailOutboxUnavailable
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var message EmailOutboxMessage
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, shop_id::text, order_id::text, event_type,
		       idempotency_key, recipient, payload::text, attempts
		FROM email_outbox
		WHERE available_at <= CURRENT_TIMESTAMP(6)
		  AND (
		      status = 'pending'
		      OR (status = 'processing' AND locked_until <= CURRENT_TIMESTAMP(6))
		  )
		ORDER BY created_at, id
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(
		&message.ID, &message.ShopID, &message.OrderID, &message.EventType,
		&message.IdempotencyKey, &message.Recipient, &message.Payload, &message.Attempts,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lockedUntil := time.Now().Add(lease)
	if _, err := tx.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = 'processing', attempts = attempts + 1, locked_until = $2
		WHERE id::text = $1
	`, message.ID, lockedUntil); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	message.Attempts++
	return &message, nil
}

func (s *Store) MarkEmailSent(ctx context.Context, id string) error {
	if s == nil || s.SQL == nil {
		return ErrEmailOutboxUnavailable
	}
	result, err := s.SQL.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = 'sent', sent_at = CURRENT_TIMESTAMP(6), locked_until = NULL, last_error = NULL
		WHERE id::text = $1 AND status = 'processing'
	`, id)
	if err != nil {
		return err
	}
	return requireOneEmailOutboxRow(result)
}

func (s *Store) RetryEmail(ctx context.Context, id string, availableAt time.Time, reason string, permanently bool) error {
	if s == nil || s.SQL == nil {
		return ErrEmailOutboxUnavailable
	}
	status := "pending"
	if permanently {
		status = "failed"
	}
	result, err := s.SQL.ExecContext(ctx, `
		UPDATE email_outbox
		SET status = $2, available_at = $3, locked_until = NULL, last_error = $4
		WHERE id::text = $1 AND status = 'processing'
	`, id, status, availableAt, boundedEmailError(reason))
	if err != nil {
		return err
	}
	return requireOneEmailOutboxRow(result)
}

func requireOneEmailOutboxRow(result sql.Result) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("email outbox row was not claimed")
	}
	return nil
}

func boundedEmailError(reason string) string {
	const maxLength = 1000
	if len(reason) > maxLength {
		return reason[:maxLength]
	}
	return reason
}

func makeOrderConfirmationPayload(order *OrderView, customerName, locale string) ([]byte, error) {
	items := make([]OrderConfirmationItem, len(order.Items))
	for index, item := range order.Items {
		items[index] = OrderConfirmationItem{
			Name: item.Name, Quantity: item.Quantity, LineTotal: item.LineTotal,
		}
	}
	return json.Marshal(OrderConfirmationPayload{
		OrderNumber: order.Number, CustomerName: customerName, Locale: locale,
		Currency: order.Currency, Subtotal: order.Subtotal,
		ShippingTotal: order.ShippingTotal, GrandTotal: order.GrandTotal,
		Items: items,
	})
}
