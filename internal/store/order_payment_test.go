package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"euromni/internal/seed"
)

func TestMemoryDevelopmentPaymentLifecycle(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	order := createMemoryPaymentOrder(t, memory, "accepted")
	if err := memory.FulfillAdminOrder(ctx, seed.ShopID, order.ID, ""); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("fulfill unpaid order error = %v, want ErrOrderTransition", err)
	}

	attempt, err := memory.CreateDevelopmentPaymentAttempt(ctx, seed.ShopID, order.ID)
	if err != nil {
		t.Fatalf("create payment attempt: %v", err)
	}
	if attempt.Amount != order.GrandTotal || attempt.Currency != order.Currency || attempt.Status != "pending" {
		t.Fatalf("attempt = %+v, want pending attempt matching the order snapshot", attempt)
	}
	if _, err := memory.CreateDevelopmentPaymentAttempt(ctx, seed.ShopID, order.ID); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("create duplicate attempt error = %v, want ErrOrderTransition", err)
	}
	result := PaymentResult{
		PaymentID: attempt.ID, OrderID: order.ID, Provider: attempt.Provider,
		Amount: attempt.Amount, Currency: attempt.Currency, Outcome: "accepted",
		EventID: "event-" + attempt.ID, Reference: "reference-" + attempt.ID,
	}
	tampered := result
	tampered.Amount = "0.0100"
	if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, tampered); !errors.Is(err, ErrInvalidPaymentResult) {
		t.Fatalf("tampered amount result error = %v, want ErrInvalidPaymentResult", err)
	}
	if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, result); err != nil {
		t.Fatalf("apply accepted result: %v", err)
	}
	if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, result); err != nil {
		t.Fatalf("replay accepted result: %v, want idempotent success", err)
	}
	adminOrder, err := memory.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil || adminOrder.Status != "paid" {
		t.Fatalf("paid order = %+v, error = %v", adminOrder, err)
	}
	if err := memory.FulfillAdminOrder(ctx, seed.ShopID, order.ID, "TRACK-DEV"); err != nil {
		t.Fatalf("fulfill paid order: %v", err)
	}
	if err := memory.FulfillAdminOrder(ctx, seed.ShopID, order.ID, "TRACK-DEV"); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("fulfill order twice error = %v, want ErrOrderTransition", err)
	}
	adminOrder, err = memory.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil || adminOrder.Status != "fulfilled" || adminOrder.ShipmentStatus != "shipped" {
		t.Fatalf("fulfilled order = %+v, error = %v", adminOrder, err)
	}
	if len(adminOrder.LifecycleEvents) != 5 {
		t.Fatalf("lifecycle events after shipping = %+v, want placed, paid, processing, prepared, shipped", adminOrder.LifecycleEvents)
	}
	if err := memory.AdvanceAdminOrderLifecycle(ctx, seed.ShopID, order.ID, "delivered", "", "carrier"); err != nil {
		t.Fatalf("record carrier delivery: %v", err)
	}
	if err := memory.AdvanceAdminOrderLifecycle(ctx, seed.ShopID, order.ID, "delivered", "", "carrier"); err != nil {
		t.Fatalf("replay carrier delivery: %v", err)
	}
	adminOrder, err = memory.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil || adminOrder.ShipmentStatus != "delivered" || len(adminOrder.LifecycleEvents) != 6 {
		t.Fatalf("delivered order lifecycle = %+v, error = %v", adminOrder, err)
	}
}

func TestMemoryOrderLifecycleRejectsOutOfOrderSteps(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	order := createMemoryPaymentOrder(t, memory, "paid")
	if err := memory.AdvanceAdminOrderLifecycle(ctx, seed.ShopID, order.ID, "prepared", "", "staff"); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("prepare before payment/processing error = %v, want ErrOrderTransition", err)
	}
	if err := memory.ConfirmManualOrderPayment(ctx, seed.ShopID, order.ID, ""); err != nil {
		t.Fatalf("confirm manual payment: %v", err)
	}
	if err := memory.AdvanceAdminOrderLifecycle(ctx, seed.ShopID, order.ID, "processing", "", "staff"); err != nil {
		t.Fatalf("start processing: %v", err)
	}
	if err := memory.AdvanceAdminOrderLifecycle(ctx, seed.ShopID, order.ID, "shipped", "TRACK", "staff"); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("ship before preparation error = %v, want ErrOrderTransition", err)
	}
}

func TestMemoryDeclinedAndAbandonedPaymentsRestoreStockOnce(t *testing.T) {
	ctx := context.Background()
	for _, outcome := range []string{"declined", "abandoned"} {
		t.Run(outcome, func(t *testing.T) {
			memory := NewMemory()
			order := createMemoryPaymentOrder(t, memory, outcome)
			attempt, err := memory.CreateDevelopmentPaymentAttempt(ctx, seed.ShopID, order.ID)
			if err != nil {
				t.Fatalf("create payment attempt: %v", err)
			}
			result := PaymentResult{
				PaymentID: attempt.ID, OrderID: order.ID, Provider: attempt.Provider,
				Amount: attempt.Amount, Currency: attempt.Currency, Outcome: outcome,
				EventID: "event-" + attempt.ID, Reference: "reference-" + attempt.ID,
			}
			if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, result); err != nil {
				t.Fatalf("apply %s result: %v", outcome, err)
			}
			if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, result); err != nil {
				t.Fatalf("replay %s result: %v, want idempotent success", outcome, err)
			}
			adminOrder, err := memory.GetAdminOrder(ctx, seed.ShopID, order.ID)
			if err != nil || adminOrder.Status != "cancelled" {
				t.Fatalf("cancelled order = %+v, error = %v", adminOrder, err)
			}
			if err := memory.FulfillAdminOrder(ctx, seed.ShopID, order.ID, ""); !errors.Is(err, ErrOrderTransition) {
				t.Fatalf("fulfill unpaid order error = %v, want ErrOrderTransition", err)
			}
			product, err := memory.GetProduct(ctx, seed.ShopID, seed.ProductID)
			if err != nil {
				t.Fatalf("read product after payment result: %v", err)
			}
			if got := product.Variants[0].Inventory.Quantity; got != 24 {
				t.Fatalf("inventory after %s result and replay = %d, want 24", outcome, got)
			}
			if err := memory.CancelAdminOrder(ctx, seed.ShopID, order.ID); !errors.Is(err, ErrOrderTransition) {
				t.Fatalf("cancel order after terminal payment result error = %v, want ErrOrderTransition", err)
			}
		})
	}
}

func TestMemoryPaymentResultRejectsUnknownOrderAndConflictingEvent(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	order := createMemoryPaymentOrder(t, memory, "invalid-event")
	attempt, err := memory.CreateDevelopmentPaymentAttempt(ctx, seed.ShopID, order.ID)
	if err != nil {
		t.Fatalf("create payment attempt: %v", err)
	}
	result := PaymentResult{
		PaymentID: attempt.ID, OrderID: order.ID, Provider: attempt.Provider,
		Amount: attempt.Amount, Currency: attempt.Currency, Outcome: "accepted",
		EventID: "reused-event", Reference: "reference-" + attempt.ID,
	}
	if err := memory.ApplyDevelopmentPaymentResult(ctx, "another-shop", result); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-shop payment result error = %v, want ErrNotFound", err)
	}
	unknownOrderResult := result
	unknownOrderResult.OrderID = "00000000-0000-4000-8000-000000000099"
	if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, unknownOrderResult); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown order payment result error = %v, want ErrNotFound", err)
	}
	otherOrder := createMemoryPaymentOrder(t, memory, "event-collision")
	otherAttempt, err := memory.CreateDevelopmentPaymentAttempt(ctx, seed.ShopID, otherOrder.ID)
	if err != nil {
		t.Fatalf("create second payment attempt: %v", err)
	}
	reused := result
	reused.PaymentID, reused.OrderID = otherAttempt.ID, otherOrder.ID
	if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, result); err != nil {
		t.Fatalf("apply first event: %v", err)
	}
	if err := memory.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, reused); !errors.Is(err, ErrInvalidPaymentResult) {
		t.Fatalf("reused event error = %v, want ErrInvalidPaymentResult", err)
	}
}

func createMemoryPaymentOrder(t *testing.T, memory *Memory, tokenLabel string) *OrderView {
	t.Helper()
	ctx := context.Background()
	tokenHash := sha256.Sum256([]byte("payment lifecycle cart " + tokenLabel))
	if err := memory.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart: %v", err)
	}
	if _, err := memory.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 2); err != nil {
		t.Fatalf("add cart line: %v", err)
	}
	order, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email: "payment@example.com", Name: "Payment test", ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout payment cart: %v", err)
	}
	return order
}
