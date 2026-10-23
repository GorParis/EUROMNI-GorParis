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

type AdminEmailOutboxEntry struct {
	ID          string     `json:"id"`
	OrderID     string     `json:"order_id"`
	OrderNumber string     `json:"order_number"`
	EventType   string     `json:"event_type"`
	Recipient   string     `json:"recipient"`
	Status      string     `json:"status"`
	Attempts    int        `json:"attempts"`
	CreatedAt   time.Time  `json:"created_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
}

type AdminEmailOutboxDetail struct {
	AdminEmailOutboxEntry
	Payload json.RawMessage `json:"payload"`
}

type EmailOutboxAdminReader interface {
	ListAdminEmails(ctx context.Context, shopID string, limit, offset int) ([]AdminEmailOutboxEntry, error)
	GetAdminEmail(ctx context.Context, shopID, emailID string) (*AdminEmailOutboxDetail, error)
}

var _ EmailOutboxAdminReader = (*Store)(nil)

func (s *Store) ListAdminEmails(ctx context.Context, shopID string, limit, offset int) ([]AdminEmailOutboxEntry, error) {
	if s == nil || s.SQL == nil {
		return nil, ErrEmailOutboxUnavailable
	}
	if limit < 1 || limit > 100 || offset < 0 || offset > 100000 {
		return nil, errors.New("invalid email outbox pagination")
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT e.id::text, e.order_id::text, COALESCE(o.number, ''), e.event_type,
		       e.recipient, e.status, e.attempts, e.created_at, e.sent_at,
		       COALESCE(e.last_error, '')
		FROM email_outbox e
		LEFT JOIN orders o ON o.id = e.order_id
		WHERE e.shop_id = $1
		ORDER BY e.created_at DESC, e.id DESC
		LIMIT $2 OFFSET $3
	`, shopID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := make([]AdminEmailOutboxEntry, 0, limit)
	for rows.Next() {
		var entry AdminEmailOutboxEntry
		var sentAt sql.NullTime
		if err := rows.Scan(
			&entry.ID, &entry.OrderID, &entry.OrderNumber, &entry.EventType,
			&entry.Recipient, &entry.Status, &entry.Attempts, &entry.CreatedAt,
			&sentAt, &entry.LastError,
		); err != nil {
			return nil, err
		}
		if sentAt.Valid {
			entry.SentAt = &sentAt.Time
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) GetAdminEmail(ctx context.Context, shopID, emailID string) (*AdminEmailOutboxDetail, error) {
	if s == nil || s.SQL == nil {
		return nil, ErrEmailOutboxUnavailable
	}
	var detail AdminEmailOutboxDetail
	var sentAt sql.NullTime
	err := s.SQL.QueryRowContext(ctx, `
		SELECT e.id::text, e.order_id::text, COALESCE(o.number, ''), e.event_type,
		       e.recipient, e.status, e.attempts, e.created_at, e.sent_at,
		       COALESCE(e.last_error, ''), e.payload
		FROM email_outbox e
		LEFT JOIN orders o ON o.id = e.order_id
		WHERE e.shop_id = $1 AND e.id::text = $2
	`, shopID, emailID).Scan(
		&detail.ID, &detail.OrderID, &detail.OrderNumber, &detail.EventType,
		&detail.Recipient, &detail.Status, &detail.Attempts, &detail.CreatedAt,
		&sentAt, &detail.LastError, &detail.Payload,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if sentAt.Valid {
		detail.SentAt = &sentAt.Time
	}
	if !json.Valid(detail.Payload) {
		return nil, errors.New("stored email payload is invalid JSON")
	}
	return &detail, nil
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
