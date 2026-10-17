package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"

	"euromni/internal/seed"
)

func TestProductReaderFromEnvFallsBackToMemory(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	r := ProductReaderFromEnv()
	if r == nil {
		t.Fatal("expected a product reader")
	}

	p, err := r.GetProduct(context.Background(), seed.ShopID, seed.ProductID)
	if err != nil {
		t.Fatalf("expected seeded product: %v", err)
	}
	if p == nil || p.Slug != "cabas-lin" {
		t.Fatalf("unexpected product payload: %#v", p)
	}

	list, err := r.ListProducts(context.Background(), seed.ShopID)
	if err != nil {
		t.Fatalf("expected catalog list: %v", err)
	}
	if len(list) != 5 || list[0].Slug != "bougie-cedre" || list[2].Slug != "ceinture-cuir" || list[4].Slug != "tasse-gres" {
		t.Fatalf("list = %+v, want five active products sorted by slug", list)
	}
	if list[2].ID != seed.SecondProductID || len(list[2].Variants) != 3 {
		t.Fatalf("belt = %+v, want three selectable sizes", list[2])
	}
	if list[3].ID != seed.ThirdProductID || len(list[3].Variants) != 3 {
		t.Fatalf("shirt = %+v, want three selectable sizes", list[3])
	}
	if len(list[0].Variants) != 1 || len(list[0].Variants[0].Options) != 0 || len(list[4].Variants[0].Options) != 0 {
		t.Fatalf("single-option products should not expose variant choices: candle=%+v cup=%+v", list[0].Variants, list[4].Variants)
	}
}

func TestMemoryCartIsolationTotalsAndInventory(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	firstToken := sha256.Sum256([]byte("first cart token"))
	secondToken := sha256.Sum256([]byte("second cart token"))
	if err := memory.CreateCart(ctx, seed.ShopID, firstToken[:]); err != nil {
		t.Fatalf("create first cart: %v", err)
	}
	if err := memory.CreateCart(ctx, seed.ShopID, secondToken[:]); err != nil {
		t.Fatalf("create second cart: %v", err)
	}

	cart, err := memory.AddCartLine(ctx, seed.ShopID, firstToken[:], seed.VariantID, 2)
	if err != nil {
		t.Fatalf("add cart line: %v", err)
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Fatalf("items = %+v, want one line with quantity 2", cart.Items)
	}
	if cart.Total.Amount != "178.0000" || cart.Total.Currency != "EUR" {
		t.Fatalf("total = %+v, want EUR 178.0000", cart.Total)
	}

	otherCart, err := memory.GetCart(ctx, seed.ShopID, secondToken[:])
	if err != nil {
		t.Fatalf("get second cart: %v", err)
	}
	if len(otherCart.Items) != 0 || otherCart.Total.Amount != "0.0000" {
		t.Fatalf("second cart = %+v, want empty cart", otherCart)
	}

	if _, err := memory.AddCartLine(ctx, seed.ShopID, firstToken[:], seed.VariantID, 23); err != ErrOutOfStock {
		t.Fatalf("overstock error = %v, want %v", err, ErrOutOfStock)
	}
	unchanged, err := memory.GetCart(ctx, seed.ShopID, firstToken[:])
	if err != nil {
		t.Fatalf("get first cart after rejected addition: %v", err)
	}
	if unchanged.Items[0].Quantity != 2 {
		t.Fatalf("quantity after rejected addition = %d, want 2", unchanged.Items[0].Quantity)
	}
}

func TestMemoryCheckoutConsumesStockAndEmptiesCart(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	tokenHash := sha256.Sum256([]byte("checkout cart token"))
	if err := memory.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart: %v", err)
	}
	if _, err := memory.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 2); err != nil {
		t.Fatalf("add cart line: %v", err)
	}

	order, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email:               "buyer@example.com",
		Name:                "Client de test",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout cart: %v", err)
	}
	if order.Status != "pending" || !strings.HasPrefix(order.Number, "EU-") {
		t.Fatalf("order = %+v, want pending order with EU number", order)
	}
	if order.Subtotal != "178.0000" || order.ShippingTotal != "6.9000" ||
		order.GrandTotal != "184.9000" || order.Currency != "EUR" ||
		order.ShippingCarrierName != "Colissimo (tarif de démonstration)" {
		t.Fatalf("order total = %+v, want EUR 184.9000 including demo shipping", order)
	}
	if len(order.Items) != 1 || order.Items[0].Name != "Cabas en lin" || order.Items[0].LineTotal != "178.0000" {
		t.Fatalf("order items = %+v, want one snapshotted cabas line", order.Items)
	}

	product, err := memory.GetProduct(ctx, seed.ShopID, seed.ProductID)
	if err != nil {
		t.Fatalf("read product after checkout: %v", err)
	}
	if product.Variants[0].Inventory.Quantity != 22 {
		t.Fatalf("remaining stock = %d, want 22", product.Variants[0].Inventory.Quantity)
	}
	cart, err := memory.GetCart(ctx, seed.ShopID, tokenHash[:])
	if err != nil {
		t.Fatalf("read cart after checkout: %v", err)
	}
	if len(cart.Items) != 0 || cart.Total.Amount != "0.0000" {
		t.Fatalf("cart after checkout = %+v, want empty cart", cart)
	}
	if _, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{}); !errors.Is(err, ErrEmptyCart) {
		t.Fatalf("second checkout error = %v, want ErrEmptyCart", err)
	}
}
