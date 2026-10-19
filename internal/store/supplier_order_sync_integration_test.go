//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/testdb"
)

func TestPostgresSupplierOrderSync(t *testing.T) {
	db := NewSQL(testdb.Start(t))
	testSQLSupplierOrderSync(t, context.Background(), db)
}

func testSQLSupplierOrderSync(t *testing.T, ctx context.Context, db *Store) {
	t.Helper()
	orderID, err := NewCatalogID()
	if err != nil {
		t.Fatalf("generate supplier sync order id: %v", err)
	}
	orderNumber := "EU-SYNC-" + orderID[:8]
	if _, err := db.SQL.ExecContext(ctx, `
		INSERT INTO orders (
			id, shop_id, number, status, currency, shipping_address,
			shipping_carrier_code, shipping_service_code
		)
		VALUES ($1, $2, $3, 'pending', 'EUR', $4::jsonb, 'test', 'test')
	`, orderID, seed.ShopID, orderNumber, `{"line1":"1 test street","city":"Paris","postal_code":"75001","country":"FR"}`); err != nil {
		t.Fatalf("insert supplier sync order: %v", err)
	}
	t.Cleanup(func() {
		if _, err := db.SQL.ExecContext(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID); err != nil {
			t.Errorf("delete supplier sync test order: %v", err)
		}
	})
	if _, err := db.SQL.ExecContext(ctx, `
		INSERT INTO order_lines (order_id, sku, name, quantity, unit_price, line_total, supplier_snapshot)
		VALUES ($1, 'TEST-1', 'Test product', 2, 5, 10, $2::jsonb)
	`, orderID, `[{"code":"selfnamed","sku":"SN-001"}]`); err != nil {
		t.Fatalf("insert supplier sync order line: %v", err)
	}
	if _, err := db.ClaimSupplierOrderSync(ctx, seed.ShopID, orderID, "selfnamed"); !errors.Is(err, ErrSupplierOrderNotReady) {
		t.Fatalf("claim pending order error = %v, want ErrSupplierOrderNotReady", err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE orders SET status = 'paid' WHERE id = $1`, orderID); err != nil {
		t.Fatalf("mark test order paid: %v", err)
	}
	first, err := db.ClaimSupplierOrderSync(ctx, seed.ShopID, orderID, "selfnamed")
	if err != nil {
		t.Fatalf("claim paid order: %v", err)
	}
	if len(first.Order.Items) != 1 || first.Order.Items[0].SupplierSKU != "SN-001" || first.Order.Items[0].Quantity != 2 {
		t.Fatalf("supplier request items = %+v", first.Order.Items)
	}
	if _, err := db.ClaimSupplierOrderSync(ctx, seed.ShopID, orderID, "selfnamed"); !errors.Is(err, ErrSupplierOrderSyncInProgress) {
		t.Fatalf("duplicate active claim error = %v, want ErrSupplierOrderSyncInProgress", err)
	}
	if err := db.FailSupplierOrderSync(ctx, seed.ShopID, orderID, "selfnamed", first.IdempotencyKey, "test failure"); err != nil {
		t.Fatalf("mark first attempt failed: %v", err)
	}
	failed, err := db.GetSupplierOrderSyncStatus(ctx, seed.ShopID, orderID, "selfnamed")
	if err != nil || failed.Status != "failed" || failed.Attempts != 1 {
		t.Fatalf("failed sync status = %+v, error = %v", failed, err)
	}
	retry, err := db.ClaimSupplierOrderSync(ctx, seed.ShopID, orderID, "selfnamed")
	if err != nil {
		t.Fatalf("claim retry: %v", err)
	}
	if retry.IdempotencyKey != first.IdempotencyKey || retry.Status.Attempts != 2 {
		t.Fatalf("retry = %+v, want stable key and second attempt", retry)
	}
	if err := db.CompleteSupplierOrderSync(ctx, seed.ShopID, orderID, "selfnamed", retry.IdempotencyKey, "mock-order-1"); err != nil {
		t.Fatalf("complete retry: %v", err)
	}
	replay, err := db.ClaimSupplierOrderSync(ctx, seed.ShopID, orderID, "selfnamed")
	if err != nil || !replay.AlreadySynced || replay.Status.ExternalOrderID != "mock-order-1" {
		t.Fatalf("replay claim = %+v, error = %v", replay, err)
	}
	adminOrder, err := db.GetAdminOrder(ctx, seed.ShopID, orderID)
	if err != nil || adminOrder.SupplierOrderSync == nil ||
		adminOrder.SupplierOrderSync.Status != "synced" ||
		adminOrder.SupplierOrderSync.ExternalOrderID != "mock-order-1" {
		t.Fatalf("admin supplier sync status = %+v, error = %v", adminOrder, err)
	}
}
