package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"euromni/internal/dbdialect"
)

const developmentPaymentProvider = "development-fake"

var _ PaymentLifecycleWriter = (*Store)(nil)

func (s *Store) CreateDevelopmentPaymentAttempt(ctx context.Context, shopID, orderID string) (*PaymentAttempt, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var amount, currency, status string
	err = tx.QueryRowContext(ctx, `
		SELECT grand_total::text, currency::text, status::text
		FROM orders
		WHERE shop_id = $1 AND id = $2
		FOR UPDATE
	`, shopID, orderID).Scan(&amount, &currency, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "pending" {
		return nil, ErrOrderTransition
	}
	var paymentCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payments WHERE order_id = $1`, orderID).Scan(&paymentCount); err != nil {
		return nil, err
	}
	if paymentCount != 0 {
		return nil, ErrOrderTransition
	}
	paymentID, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO payments (id, order_id, provider, status, amount, currency)
		VALUES ($1, $2, $3, 'pending', $4, $5)
	`, paymentID, orderID, developmentPaymentProvider, amount, currency); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &PaymentAttempt{
		ID: paymentID, OrderID: orderID, Provider: developmentPaymentProvider,
		Amount: amount, Currency: currency, Status: "pending",
	}, nil
}

func (s *Store) ApplyDevelopmentPaymentResult(ctx context.Context, shopID string, result PaymentResult) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	paymentStatus, orderStatus, valid := DevelopmentPaymentStatuses(result.Outcome)
	if !valid || result.Provider != developmentPaymentProvider ||
		result.PaymentID == "" || result.OrderID == "" || result.EventID == "" ||
		result.Amount == "" || result.Currency == "" {
		return ErrInvalidPaymentResult
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var paymentOrderID, provider, currentPaymentStatus, amount, currency, eventID, reference string
	var orderStatusNow, orderAmount, orderCurrency string
	err = tx.QueryRowContext(ctx, `
		SELECT p.order_id::text, p.provider, p.status::text, p.amount::text, p.currency::text,
		       COALESCE(p.provider_event_id, ''), COALESCE(p.provider_ref, ''),
		       o.status::text, o.grand_total::text, o.currency::text
		FROM payments p
		JOIN orders o ON o.id = p.order_id
		WHERE o.shop_id = $1 AND p.id = $2
		FOR UPDATE
	`, shopID, result.PaymentID).Scan(
		&paymentOrderID, &provider, &currentPaymentStatus, &amount, &currency, &eventID, &reference,
		&orderStatusNow, &orderAmount, &orderCurrency,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if paymentOrderID != result.OrderID || provider != developmentPaymentProvider ||
		amount != result.Amount || currency != result.Currency ||
		orderAmount != result.Amount || orderCurrency != result.Currency {
		return ErrInvalidPaymentResult
	}
	if currentPaymentStatus != "pending" {
		if eventID == result.EventID && currentPaymentStatus == paymentStatus &&
			reference == result.Reference && orderStatusNow == orderStatus {
			return nil
		}
		return ErrOrderTransition
	}
	if orderStatusNow != "pending" {
		return ErrOrderTransition
	}
	var duplicateEventCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM payments WHERE provider_event_id = $1
	`, result.EventID).Scan(&duplicateEventCount); err != nil {
		return err
	}
	if duplicateEventCount != 0 {
		return ErrInvalidPaymentResult
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE payments
		SET status = $2, provider_event_id = $3, provider_ref = NULLIF($4, ''), updated_at = now()
		WHERE id = $1
	`, result.PaymentID, paymentStatus, result.EventID, result.Reference); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = $2, updated_at = now()
		WHERE shop_id = $1 AND id = $3
	`, shopID, orderStatus, result.OrderID); err != nil {
		return err
	}
	if orderStatus == "paid" {
		if err := recordOrderLifecycleEvent(ctx, tx, s.Dialect, shopID, result.OrderID, "paid", "payment", ""); err != nil {
			return err
		}
	}
	if orderStatus == "cancelled" {
		if err := restockCancelledOrder(ctx, tx, s.Dialect, result.OrderID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func DevelopmentPaymentStatuses(outcome string) (paymentStatus, orderStatus string, ok bool) {
	switch outcome {
	case "accepted":
		return "captured", "paid", true
	case "declined":
		return "failed", "cancelled", true
	case "abandoned":
		return "cancelled", "cancelled", true
	default:
		return "", "", false
	}
}

func restockCancelledOrder(ctx context.Context, tx *sql.Tx, dialect dbdialect.Dialect, orderID string) error {
	if dialect == dbdialect.MariaDB {
		_, err := tx.ExecContext(ctx, `
			UPDATE inventory_items inventory
			JOIN (
				SELECT variant_id, SUM(quantity) AS quantity
				FROM order_lines
				WHERE order_id = $1 AND variant_id IS NOT NULL
				GROUP BY variant_id
			) order_line_totals ON inventory.variant_id = order_line_totals.variant_id
			SET inventory.quantity = inventory.quantity + order_line_totals.quantity, inventory.updated_at = now()
			WHERE inventory.track = true
		`, orderID)
		return err
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE inventory_items inventory
		SET quantity = inventory.quantity + lines.quantity, updated_at = now()
		FROM (
			SELECT variant_id, sum(quantity)::integer AS quantity
			FROM order_lines
			WHERE order_id = $1 AND variant_id IS NOT NULL
			GROUP BY variant_id
		) lines
		WHERE inventory.variant_id = lines.variant_id AND inventory.track = true
	`, orderID)
	if err != nil {
		return fmt.Errorf("restore inventory for cancelled order: %w", err)
	}
	return nil
}
