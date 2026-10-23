package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"euromni/internal/dbdialect"
)

func (s *Store) AdvanceAdminOrderLifecycle(ctx context.Context, shopID, orderID, eventType, tracking, source string) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	if !validOrderLifecycleEvent(eventType) || !validOrderLifecycleSource(source) {
		return ErrOrderTransition
	}

	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status, carrier string
	err = tx.QueryRowContext(ctx, `
		SELECT status::text, COALESCE(shipping_carrier_name, '')
		FROM orders
		WHERE shop_id = $1 AND id = $2
		FOR UPDATE
	`, shopID, orderID).Scan(&status, &carrier)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var eventExists bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM order_lifecycle_events
			WHERE shop_id = $1 AND order_id = $2 AND event_type = $3
		)
	`, shopID, orderID, eventType).Scan(&eventExists); err != nil {
		return err
	}
	if eventExists {
		return nil
	}

	switch eventType {
	case "processing":
		if status != "paid" {
			return ErrOrderTransition
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE orders SET status = 'fulfilling', updated_at = now()
			WHERE shop_id = $1 AND id = $2
		`, shopID, orderID); err != nil {
			return err
		}
	case "prepared":
		if status != "fulfilling" || carrier == "" {
			return ErrOrderTransition
		}
		shipmentID, shipmentStatus, err := lockLatestShipment(ctx, tx, orderID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO shipments (order_id, status, carrier)
				VALUES ($1, 'ready', $2)
			`, orderID, carrier); err != nil {
				return err
			}
		case shipmentStatus == "pending":
			if _, err := tx.ExecContext(ctx, `
				UPDATE shipments SET status = 'ready', updated_at = now()
				WHERE id::text = $1
			`, shipmentID); err != nil {
				return err
			}
		case shipmentStatus != "ready":
			return ErrOrderTransition
		}
	case "shipped":
		if status != "fulfilling" {
			return ErrOrderTransition
		}
		shipmentID, shipmentStatus, err := lockLatestShipment(ctx, tx, orderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrOrderTransition
			}
			return err
		}
		if shipmentStatus != "ready" {
			return ErrOrderTransition
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE shipments
			SET status = 'shipped', tracking = NULLIF($2, ''), shipped_at = now(), updated_at = now()
			WHERE id::text = $1
		`, shipmentID, tracking); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE orders SET status = 'fulfilled', updated_at = now()
			WHERE shop_id = $1 AND id = $2
		`, shopID, orderID); err != nil {
			return err
		}
	case "delivered":
		if status != "fulfilled" {
			return ErrOrderTransition
		}
		shipmentID, shipmentStatus, err := lockLatestShipment(ctx, tx, orderID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrOrderTransition
			}
			return err
		}
		if shipmentStatus != "shipped" {
			return ErrOrderTransition
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE shipments SET status = 'delivered', updated_at = now()
			WHERE id::text = $1
		`, shipmentID); err != nil {
			return err
		}
	default:
		return ErrOrderTransition
	}

	if err := recordOrderLifecycleEvent(ctx, tx, s.Dialect, shopID, orderID, eventType, source, tracking); err != nil {
		return err
	}
	return tx.Commit()
}

func lockLatestShipment(ctx context.Context, tx *sql.Tx, orderID string) (id, status string, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT id::text, status::text FROM shipments
		WHERE order_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
		FOR UPDATE
	`, orderID).Scan(&id, &status)
	return id, status, err
}

func recordOrderLifecycleEvent(
	ctx context.Context, tx *sql.Tx, dialect dbdialect.Dialect,
	shopID, orderID, eventType, source, tracking string,
) error {
	if !validOrderLifecycleEvent(eventType) || !validOrderLifecycleSource(source) {
		return ErrOrderTransition
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO order_lifecycle_events (shop_id, order_id, event_type, source, tracking)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
	`, shopID, orderID, eventType, source, tracking); err != nil {
		return err
	}
	if eventType == "order_placed" {
		return nil
	}

	var payload OrderStatusPayload
	err := tx.QueryRowContext(ctx, `
		SELECT o.number, c.name, s.default_locale
		FROM orders o
		JOIN customers c ON c.id = o.customer_id
		JOIN shops s ON s.id = o.shop_id
		WHERE o.shop_id = $1 AND o.id = $2
	`, shopID, orderID).Scan(&payload.OrderNumber, &payload.CustomerName, &payload.Locale)
	if err != nil {
		return err
	}
	payload.Status = eventType
	payload.Tracking = tracking
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode order lifecycle email: %w", err)
	}
	query := `
		INSERT INTO email_outbox (shop_id, order_id, event_type, idempotency_key, recipient, payload)
		SELECT $1, $2, 'order_status', $3, c.email, $4
		FROM customers c JOIN orders o ON o.customer_id = c.id
		WHERE o.shop_id = $1 AND o.id = $2
	`
	if dialect != dbdialect.MariaDB {
		query = `
			INSERT INTO email_outbox (shop_id, order_id, event_type, idempotency_key, recipient, payload)
			SELECT $1, $2, 'order_status', $3, c.email, $4::jsonb
			FROM customers c JOIN orders o ON o.customer_id = c.id
			WHERE o.shop_id = $1 AND o.id = $2
		`
	}
	result, err := tx.ExecContext(ctx, query,
		shopID, orderID, "order-status:"+orderID+":"+eventType, string(encoded))
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("order lifecycle email was not queued")
	}
	return nil
}

func validOrderLifecycleEvent(eventType string) bool {
	switch eventType {
	case "order_placed", "paid", "processing", "prepared", "shipped", "delivered":
		return true
	default:
		return false
	}
}

func validOrderLifecycleSource(source string) bool {
	switch source {
	case "checkout", "payment", "staff", "supplier", "carrier", "local_demo":
		return true
	default:
		return false
	}
}
