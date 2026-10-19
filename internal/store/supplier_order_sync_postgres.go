package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"euromni/internal/dbdialect"
)

var _ SupplierOrderSyncStore = (*Store)(nil)

func (s *Store) ClaimSupplierOrderSync(ctx context.Context, shopID, orderID, supplierCode string) (*SupplierOrderSyncClaim, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	if supplierCode == "" {
		return nil, errors.New("supplier code is required")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var order SupplierOrderRequest
	var status, addressJSON string
	err = tx.QueryRowContext(ctx, `
		SELECT o.id::text, o.number, o.status::text, o.currency::text,
		       COALESCE(o.shipping_address::text, '{}'), COALESCE(c.name, ''), COALESCE(c.email::text, ''),
		       COALESCE(o.shipping_carrier_code, ''), COALESCE(o.shipping_service_code, '')
		FROM orders o
		LEFT JOIN customers c ON c.id = o.customer_id
		WHERE o.shop_id = $1 AND o.id = $2
		FOR UPDATE OF o
	`, shopID, orderID).Scan(
		&order.OrderID, &order.OrderNumber, &status, &order.Currency,
		&addressJSON, &order.BuyerName, &order.BuyerEmail,
		&order.ShippingCode, &order.ShippingGroup,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "paid" {
		return nil, ErrSupplierOrderNotReady
	}
	if addressJSON != "" && addressJSON != "null" {
		if err := json.Unmarshal([]byte(addressJSON), &order.Shipping); err != nil {
			return nil, fmt.Errorf("decode supplier order shipping address: %w", err)
		}
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT name, quantity, COALESCE(supplier_snapshot::text, '[]')
		FROM order_lines
		WHERE order_id = $1
		ORDER BY id
	`, orderID)
	if err != nil {
		return nil, err
	}
	order.Items = make([]SupplierOrderItem, 0)
	for rows.Next() {
		var name, snapshotJSON string
		var quantity int
		if err := rows.Scan(&name, &quantity, &snapshotJSON); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var snapshot []SupplierSnapshotItem
		if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("decode supplier snapshot for order line: %w", err)
		}
		for _, supplier := range snapshot {
			if supplier.Code == supplierCode {
				order.Items = append(order.Items, SupplierOrderItem{
					SupplierSKU: supplier.SKU, Name: name, Quantity: quantity,
				})
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(order.Items) == 0 {
		return nil, ErrSupplierOrderHasNoItems
	}

	idempotencyKey := SupplierOrderIdempotencyKey(shopID, orderID, supplierCode)
	leaseDeadline := "now() + interval '2 minutes'"
	if s.Dialect == dbdialect.MariaDB {
		leaseDeadline = "now() + INTERVAL 2 MINUTE"
	}
	var syncStatus, externalOrderID, lastError string
	var attempts int
	var leaseActive bool
	err = tx.QueryRowContext(ctx, `
		SELECT status, COALESCE(external_order_id, ''), attempts, COALESCE(last_error, ''),
		       COALESCE(locked_until > now(), false)
		FROM supplier_order_syncs
		WHERE order_id = $1 AND supplier_code = $2
		FOR UPDATE
	`, orderID, supplierCode).Scan(&syncStatus, &externalOrderID, &attempts, &lastError, &leaseActive)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
			INSERT INTO supplier_order_syncs (
				order_id, supplier_code, status, idempotency_key, attempts, locked_until
			)
			VALUES ($1, $2, 'syncing', $3, 1, %s)
		`, leaseDeadline), orderID, supplierCode, idempotencyKey)
		if err != nil {
			return nil, err
		}
		attempts = 1
	} else if err != nil {
		return nil, err
	} else if syncStatus == "synced" {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &SupplierOrderSyncClaim{
			IdempotencyKey: idempotencyKey,
			Status: SupplierOrderSyncStatus{
				OrderID: orderID, SupplierCode: supplierCode, Status: syncStatus,
				ExternalOrderID: externalOrderID, Attempts: attempts, LastError: lastError,
			},
			AlreadySynced: true,
		}, nil
	} else {
		if syncStatus == "syncing" && leaseActive {
			return nil, ErrSupplierOrderSyncInProgress
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`
			UPDATE supplier_order_syncs
			SET status = 'syncing', attempts = attempts + 1, last_error = NULL,
			    locked_until = %s, updated_at = now()
			WHERE order_id = $1 AND supplier_code = $2
		`, leaseDeadline), orderID, supplierCode); err != nil {
			return nil, err
		}
		attempts++
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &SupplierOrderSyncClaim{
		IdempotencyKey: idempotencyKey,
		Status: SupplierOrderSyncStatus{
			OrderID: orderID, SupplierCode: supplierCode,
			Status: "syncing", Attempts: attempts,
		},
		Order: order,
	}, nil
}

func (s *Store) CompleteSupplierOrderSync(ctx context.Context, shopID, orderID, supplierCode, idempotencyKey, externalOrderID string) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	externalOrderID = strings.TrimSpace(externalOrderID)
	if externalOrderID == "" {
		return errors.New("external order id is required")
	}
	query := `
		UPDATE supplier_order_syncs sync
		SET status = 'synced', external_order_id = $5, last_error = NULL,
		    locked_until = NULL, updated_at = now()
		FROM orders o
		WHERE sync.order_id = o.id AND o.shop_id = $1 AND sync.order_id = $2
		  AND sync.supplier_code = $3 AND sync.idempotency_key = $4 AND sync.status = 'syncing'
	`
	args := []any{shopID, orderID, supplierCode, idempotencyKey, externalOrderID}
	if s.Dialect == dbdialect.MariaDB {
		query = `
			UPDATE supplier_order_syncs sync
			JOIN orders o ON sync.order_id = o.id
			SET sync.status = 'synced', sync.external_order_id = $5, sync.last_error = NULL,
			    sync.locked_until = NULL, sync.updated_at = now()
			WHERE o.shop_id = $1 AND sync.order_id = $2 AND sync.supplier_code = $3
			  AND sync.idempotency_key = $4 AND sync.status = 'syncing'
		`
	}
	result, err := s.SQL.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrOrderTransition
	}
	return nil
}

func (s *Store) FailSupplierOrderSync(ctx context.Context, shopID, orderID, supplierCode, idempotencyKey, reason string) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	reason = boundedSupplierSyncError(reason)
	query := `
		UPDATE supplier_order_syncs sync
		SET status = 'failed', last_error = NULLIF($5, ''),
		    locked_until = NULL, updated_at = now()
		FROM orders o
		WHERE sync.order_id = o.id AND o.shop_id = $1 AND sync.order_id = $2
		  AND sync.supplier_code = $3 AND sync.idempotency_key = $4 AND sync.status = 'syncing'
	`
	args := []any{shopID, orderID, supplierCode, idempotencyKey, reason}
	if s.Dialect == dbdialect.MariaDB {
		query = `
			UPDATE supplier_order_syncs sync
			JOIN orders o ON sync.order_id = o.id
			SET sync.status = 'failed', sync.last_error = NULLIF($5, ''),
			    sync.locked_until = NULL, sync.updated_at = now()
			WHERE o.shop_id = $1 AND sync.order_id = $2 AND sync.supplier_code = $3
			  AND sync.idempotency_key = $4 AND sync.status = 'syncing'
		`
	}
	result, err := s.SQL.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return ErrOrderTransition
	}
	return nil
}

func (s *Store) GetSupplierOrderSyncStatus(ctx context.Context, shopID, orderID, supplierCode string) (*SupplierOrderSyncStatus, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	var status SupplierOrderSyncStatus
	err := s.SQL.QueryRowContext(ctx, `
		SELECT sync.order_id::text, sync.supplier_code, sync.status,
		       COALESCE(sync.external_order_id, ''), sync.attempts, COALESCE(sync.last_error, '')
		FROM supplier_order_syncs sync
		JOIN orders o ON o.id = sync.order_id
		WHERE o.shop_id = $1 AND sync.order_id = $2 AND sync.supplier_code = $3
	`, shopID, orderID, supplierCode).Scan(
		&status.OrderID, &status.SupplierCode, &status.Status,
		&status.ExternalOrderID, &status.Attempts, &status.LastError,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &status, nil
}
