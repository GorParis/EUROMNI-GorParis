package store

import (
	"context"
	"errors"
	"testing"

	"euromni/internal/seed"
)

func TestMemorySupplierOrderSyncRequiresPaidOrderAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	order := createMemoryPaymentOrder(t, memory, "supplier-sync")

	memory.mu.Lock()
	memory.orders[order.ID].view.Items[0].SupplierSnapshot = []SupplierSnapshotItem{
		{Code: "selfnamed", SKU: "SN-001"},
	}
	memory.mu.Unlock()
	if _, err := memory.ClaimSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed"); !errors.Is(err, ErrSupplierOrderNotReady) {
		t.Fatalf("claim unpaid order error = %v, want ErrSupplierOrderNotReady", err)
	}

	if err := memory.ConfirmManualOrderPayment(ctx, seed.ShopID, order.ID, "BANK-1"); err != nil {
		t.Fatalf("confirm payment: %v", err)
	}
	claim, err := memory.ClaimSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed")
	if err != nil {
		t.Fatalf("claim paid order: %v", err)
	}
	if claim.Order.OrderID != order.ID || claim.Order.BuyerEmail != "payment@example.com" ||
		len(claim.Order.Items) != 1 || claim.Order.Items[0].SupplierSKU != "SN-001" ||
		claim.Order.Items[0].Quantity != 2 {
		t.Fatalf("supplier order request = %+v", claim.Order)
	}
	if _, err := memory.ClaimSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed"); !errors.Is(err, ErrSupplierOrderSyncInProgress) {
		t.Fatalf("duplicate active claim error = %v, want ErrSupplierOrderSyncInProgress", err)
	}

	if err := memory.CompleteSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed", claim.IdempotencyKey, "mock-123"); err != nil {
		t.Fatalf("complete supplier sync: %v", err)
	}
	replay, err := memory.ClaimSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed")
	if err != nil {
		t.Fatalf("claim already-synced order: %v", err)
	}
	if !replay.AlreadySynced || replay.IdempotencyKey != claim.IdempotencyKey ||
		replay.Status.ExternalOrderID != "mock-123" || replay.Status.Attempts != 1 {
		t.Fatalf("replay claim = %+v, want same completed sync and idempotency key", replay)
	}
	adminOrder, err := memory.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil {
		t.Fatalf("read admin order: %v", err)
	}
	if adminOrder.SupplierOrderSync == nil || adminOrder.SupplierOrderSync.Status != "synced" {
		t.Fatalf("admin supplier sync status = %+v, want synced", adminOrder.SupplierOrderSync)
	}
}

func TestMemorySupplierOrderSyncRetryReusesIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	order := createMemoryPaymentOrder(t, memory, "supplier-sync-retry")
	memory.mu.Lock()
	memory.orders[order.ID].view.Items[0].SupplierSnapshot = []SupplierSnapshotItem{
		{Code: "selfnamed", SKU: "SN-002"},
	}
	memory.mu.Unlock()
	if err := memory.ConfirmManualOrderPayment(ctx, seed.ShopID, order.ID, "BANK-2"); err != nil {
		t.Fatalf("confirm payment: %v", err)
	}
	first, err := memory.ClaimSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if err := memory.FailSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed", first.IdempotencyKey, "mock failure"); err != nil {
		t.Fatalf("record failed sync: %v", err)
	}
	retry, err := memory.ClaimSupplierOrderSync(ctx, seed.ShopID, order.ID, "selfnamed")
	if err != nil {
		t.Fatalf("retry claim: %v", err)
	}
	if retry.IdempotencyKey != first.IdempotencyKey || retry.Status.Attempts != 2 {
		t.Fatalf("retry claim = %+v, want stable key and second attempt", retry)
	}
}

func TestMemorySupplierOrderSyncDoesNotExposeUnknownOrders(t *testing.T) {
	memory := NewMemory()
	if _, err := memory.ClaimSupplierOrderSync(context.Background(), seed.ShopID, "missing", "selfnamed"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("claim missing order error = %v, want ErrNotFound", err)
	}
}
