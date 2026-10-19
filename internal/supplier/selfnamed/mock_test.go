package selfnamed

import (
	"context"
	"testing"

	"euromni/internal/store"
)

func TestMockSubmitOrderIsIdempotentAndLocal(t *testing.T) {
	request := store.SupplierOrderRequest{
		OrderID: "order-1",
		Items:   []store.SupplierOrderItem{{SupplierSKU: "SN-001", Quantity: 2}},
	}
	provider := Mock{}
	first, err := provider.SubmitOrder(context.Background(), request, "stable-key")
	if err != nil {
		t.Fatalf("first SubmitOrder() error = %v", err)
	}
	second, err := provider.SubmitOrder(context.Background(), request, "stable-key")
	if err != nil {
		t.Fatalf("repeated SubmitOrder() error = %v", err)
	}
	if first == "" || first != second {
		t.Fatalf("external order IDs = %q and %q, want the same deterministic ID", first, second)
	}
}

func TestMockRejectsInvalidOrder(t *testing.T) {
	if _, err := (Mock{}).SubmitOrder(context.Background(), store.SupplierOrderRequest{OrderID: "order-1"}, "key"); err == nil {
		t.Fatal("SubmitOrder() error = nil, want invalid order error")
	}
}
