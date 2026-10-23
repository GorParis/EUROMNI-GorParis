package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var _ OrderAdminReader = (*Store)(nil)

func (s *Store) ListAdminOrders(ctx context.Context, shopID string, limit, offset int) ([]AdminOrderView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT id::text
		FROM orders
		WHERE shop_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, shopID, limit, offset)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	orders := make([]AdminOrderView, 0, len(ids))
	for _, id := range ids {
		order, err := s.GetAdminOrder(ctx, shopID, id)
		if err != nil {
			return nil, err
		}
		orders = append(orders, *order)
	}
	return orders, nil
}

func (s *Store) GetAdminOrder(ctx context.Context, shopID, orderID string) (*AdminOrderView, error) {
	if s == nil || s.SQL == nil {
		return nil, errors.New("store not configured")
	}
	var order AdminOrderView
	var address sql.NullString
	var weightGrams sql.NullInt64
	var pricingSubtotal sql.NullString
	var rateRule sql.NullString
	var policySnapshot sql.NullString
	err := s.SQL.QueryRowContext(ctx, `
		SELECT o.id::text, o.number, o.status::text, o.currency::text,
		       o.subtotal::text, o.tax_total::text, o.shipping_total::text, o.grand_total::text,
		       COALESCE(o.shipping_carrier_code, ''), COALESCE(o.shipping_carrier_name, ''),
		       COALESCE(o.shipping_service_code, ''), COALESCE(o.shipping_service_name, ''),
		       COALESCE(o.shipping_rate_version::text, ''),
		       COALESCE(o.shipping_zone_code, ''), COALESCE(o.shipping_zone_name, ''),
		       o.shipping_rate_mode, o.shipping_weight_grams,
		       o.shipping_pricing_subtotal::text, o.shipping_rate_rule::text,
		       o.shipping_policy_snapshot::text,
		       COALESCE((SELECT shipment.status::text FROM shipments shipment
		                 WHERE shipment.order_id = o.id ORDER BY shipment.created_at DESC, shipment.id DESC LIMIT 1), ''),
		       COALESCE((SELECT shipment.tracking FROM shipments shipment
		                 WHERE shipment.order_id = o.id ORDER BY shipment.created_at DESC, shipment.id DESC LIMIT 1), ''),
		       COALESCE(c.email::text, ''), COALESCE(c.name, ''),
		       o.shipping_address::text, o.created_at
		FROM orders o
		LEFT JOIN customers c ON c.id = o.customer_id
		WHERE o.shop_id = $1 AND o.id = $2
	`, shopID, orderID).Scan(
		&order.ID, &order.Number, &order.Status, &order.Currency,
		&order.Subtotal, &order.TaxTotal, &order.ShippingTotal, &order.GrandTotal,
		&order.ShippingCarrierCode, &order.ShippingCarrierName,
		&order.ShippingServiceCode, &order.ShippingServiceName, &order.ShippingRateVersion,
		&order.ShippingZoneCode, &order.ShippingZoneName,
		&order.ShippingRateMode, &weightGrams, &pricingSubtotal, &rateRule, &policySnapshot,
		&order.ShipmentStatus, &order.Tracking,
		&order.Email, &order.Name, &address, &order.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if address.Valid && address.String != "" {
		if err := json.Unmarshal([]byte(address.String), &order.ShippingAddress); err != nil {
			return nil, err
		}
		if weightGrams.Valid {
			order.ShippingWeightGrams = &weightGrams.Int64
		}
		if pricingSubtotal.Valid {
			order.ShippingPricingSubtotal = &pricingSubtotal.String
		}
		if rateRule.Valid {
			order.ShippingRateRule = json.RawMessage(rateRule.String)
		}
		if policySnapshot.Valid && policySnapshot.String != "{}" {
			var snapshot ShippingPricingPolicySnapshot
			if err := json.Unmarshal([]byte(policySnapshot.String), &snapshot); err != nil {
				return nil, err
			}
			order.ShippingPolicySnapshot = &snapshot
		}
	}
	rows, err := s.SQL.QueryContext(ctx, `
		SELECT sku, name, quantity, unit_price::text, line_total::text, personalization_snapshot::text
		FROM order_lines
		WHERE order_id = $1
		ORDER BY id
	`, orderID)
	if err != nil {
		return nil, err
	}
	order.Items = make([]OrderLineView, 0)
	for rows.Next() {
		var item OrderLineView
		var personalizationJSON string
		if err := rows.Scan(&item.SKU, &item.Name, &item.Quantity, &item.UnitPrice, &item.LineTotal, &personalizationJSON); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal([]byte(personalizationJSON), &item.Personalizations); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if item.Personalizations == nil {
			item.Personalizations = []PersonalizationSelection{}
		}
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = s.SQL.QueryContext(ctx, `
		SELECT event_type, source, COALESCE(tracking, ''), occurred_at
		FROM order_lifecycle_events
		WHERE shop_id = $1 AND order_id = $2
		ORDER BY occurred_at, id
	`, shopID, orderID)
	if err != nil {
		return nil, err
	}
	order.LifecycleEvents = make([]OrderLifecycleEvent, 0)
	for rows.Next() {
		var event OrderLifecycleEvent
		if err := rows.Scan(&event.EventType, &event.Source, &event.Tracking, &event.OccurredAt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		order.LifecycleEvents = append(order.LifecycleEvents, event)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if syncStatus, err := s.GetSupplierOrderSyncStatus(ctx, shopID, orderID, "selfnamed"); err == nil {
		order.SupplierOrderSync = syncStatus
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return &order, nil
}

func (s *Store) ConfirmManualOrderPayment(ctx context.Context, shopID, orderID, providerReference string) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status, currency string
	var amount string
	err = tx.QueryRowContext(ctx, `
		SELECT status::text, grand_total::text, currency::text
		FROM orders
		WHERE shop_id = $1 AND id = $2
		FOR UPDATE
	`, shopID, orderID).Scan(&status, &amount, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "pending" {
		return ErrOrderTransition
	}
	var paymentCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payments WHERE order_id = $1`, orderID).Scan(&paymentCount); err != nil {
		return err
	}
	if paymentCount != 0 {
		return ErrOrderTransition
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO payments (order_id, provider, status, amount, currency, provider_ref)
		VALUES ($1, 'manual', 'captured', $2, $3, NULLIF($4, ''))
	`, orderID, amount, currency, providerReference); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = 'paid', updated_at = now()
		WHERE shop_id = $1 AND id = $2
	`, shopID, orderID); err != nil {
		return err
	}
	if err := recordOrderLifecycleEvent(ctx, tx, s.Dialect, shopID, orderID, "paid", "payment", ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FulfillAdminOrder(ctx context.Context, shopID, orderID, tracking string) error {
	order, err := s.GetAdminOrder(ctx, shopID, orderID)
	if err != nil {
		return err
	}
	if order.Status != "paid" && order.Status != "fulfilling" {
		return ErrOrderTransition
	}
	if err := s.AdvanceAdminOrderLifecycle(ctx, shopID, orderID, "processing", "", "staff"); err != nil {
		if !errors.Is(err, ErrOrderTransition) {
			return err
		}
	}
	if err := s.AdvanceAdminOrderLifecycle(ctx, shopID, orderID, "prepared", "", "staff"); err != nil {
		if !errors.Is(err, ErrOrderTransition) {
			return err
		}
	}
	return s.AdvanceAdminOrderLifecycle(ctx, shopID, orderID, "shipped", tracking, "staff")
}

func (s *Store) CancelAdminOrder(ctx context.Context, shopID, orderID string) error {
	if s == nil || s.SQL == nil {
		return errors.New("store not configured")
	}
	tx, err := s.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT status::text FROM orders
		WHERE shop_id = $1 AND id = $2
		FOR UPDATE
	`, shopID, orderID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "pending" {
		return ErrOrderTransition
	}
	var paymentCount int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM payments WHERE order_id = $1`, orderID).Scan(&paymentCount); err != nil {
		return err
	}
	if paymentCount != 0 {
		return ErrOrderTransition
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = 'cancelled', updated_at = now()
		WHERE shop_id = $1 AND id = $2
	`, shopID, orderID); err != nil {
		return err
	}
	if err := restockCancelledOrder(ctx, tx, s.Dialect, orderID); err != nil {
		return err
	}
	return tx.Commit()
}
