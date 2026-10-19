package wasm

import (
	"context"
	"crypto/sha256"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/store"
)

func TestBundledFreeShippingPolicyRunsWasmRule(t *testing.T) {
	policy, err := NewBundledFreeShippingPolicy()
	if err != nil {
		t.Fatalf("create bundled policy: %v", err)
	}
	t.Cleanup(policy.Close)

	tests := []struct {
		name       string
		baseAmount string
		subtotal   string
		want       string
	}{
		{name: "below threshold", baseAmount: "6.9000", subtotal: "99.9999", want: "6.9000"},
		{name: "at threshold", baseAmount: "6.9000", subtotal: "100.0000", want: "0.0000"},
		{name: "above threshold", baseAmount: "4.5000", subtotal: "120.0000", want: "0.0000"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			amount, snapshot, err := policy.Evaluate(context.Background(), testCase.baseAmount, testCase.subtotal)
			if err != nil {
				t.Fatalf("evaluate policy: %v", err)
			}
			if amount != testCase.want {
				t.Fatalf("amount = %q, want %q", amount, testCase.want)
			}
			if snapshot.ModuleName != freeShippingModuleName ||
				snapshot.ModuleVersion != freeShippingModuleVersion ||
				snapshot.ContractVersion != "euromni.shipping.v1" ||
				len(snapshot.ModuleSHA256) != 64 {
				t.Fatalf("module snapshot = %+v", snapshot)
			}
		})
	}
}

func TestBundledFreeShippingPolicyFlowsThroughCartQuoteAndCheckout(t *testing.T) {
	ctx := context.Background()
	policy, err := NewBundledFreeShippingPolicy()
	if err != nil {
		t.Fatalf("create bundled policy: %v", err)
	}
	t.Cleanup(policy.Close)
	memory := store.NewMemory()
	memory.SetShippingPricingPolicy(policy)
	tokenHash := sha256.Sum256([]byte("bundled wasm shipping policy integration"))
	if err := memory.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart: %v", err)
	}
	if _, err := memory.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 2); err != nil {
		t.Fatalf("add cart line: %v", err)
	}
	options, err := memory.ListCartShippingOptions(ctx, seed.ShopID, tokenHash[:], "FR")
	if err != nil || len(options) != 1 || options[0].Amount != "0.0000" ||
		options[0].PolicySnapshot == nil || options[0].PolicySnapshot.ModuleName != freeShippingModuleName {
		t.Fatalf("quote = %+v, error = %v", options, err)
	}
	order, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], store.CheckoutDetails{
		Email: "wasm-policy@example.com", Name: "Wasm Policy",
		ShippingCarrierCode: "colissimo", ShippingServiceCode: "legacy",
		ExpectedShippingTotal: options[0].Amount, ExpectedShippingQuoteID: options[0].QuoteID,
		ShippingAddress: store.ShippingAddress{
			Line1: "1 rue Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil || order.ShippingTotal != "0.0000" ||
		order.ShippingPolicySnapshot == nil ||
		order.ShippingPolicySnapshot.ModuleSHA256 != options[0].PolicySnapshot.ModuleSHA256 {
		t.Fatalf("order = %+v, error = %v", order, err)
	}
}

func TestBundledFreeShippingPolicyRejectsAmountsOutsideIntegerRange(t *testing.T) {
	policy, err := NewBundledFreeShippingPolicy()
	if err != nil {
		t.Fatalf("create bundled policy: %v", err)
	}
	t.Cleanup(policy.Close)
	if _, _, err := policy.Evaluate(context.Background(), "1000000000000000.0000", "10.0000"); err == nil {
		t.Fatal("policy accepted an amount outside its exact integer range")
	}
}
