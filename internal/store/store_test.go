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

func TestOpenRejectsPostgreSQLSimpleProtocol(t *testing.T) {
	_, err := Open("postgres://user:password@localhost:5432/euromni?default_query_exec_mode=simple_protocol")
	if err == nil || !strings.Contains(err.Error(), "simple protocol is disabled") {
		t.Fatalf("Open() error = %v, want simple protocol rejection", err)
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

func TestMemoryCatalogReferencesAndPersonalizationSnapshots(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	brand, err := memory.CreateBrand(ctx, seed.ShopID, BrandInput{Slug: "atelier-test", Name: "Atelier Test"})
	if err != nil {
		t.Fatalf("create brand: %v", err)
	}
	supplier, err := memory.CreateSupplier(ctx, seed.ShopID, SupplierInput{Code: "FOURN-1", Name: "Fournisseur 1", Active: true})
	if err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	options := []PersonalizationOption{{
		Code: "emballage", Label: "Emballage cadeau", Required: true,
		Choices: []PersonalizationChoice{
			{Value: "gift", Label: "Avec papier cadeau"},
			{Value: "plain", Label: "Sans papier cadeau"},
		},
	}}
	product, err := memory.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug: "objet-personnalisable", Status: "active", BrandID: brand.ID,
		SupplierIDs: []string{supplier.ID}, PersonalizationOptions: options,
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Objet personnalisable"}},
		Variants: []VariantInput{{
			SKU: "OBJET-1", Options: map[string]any{}, Prices: []PriceView{{Currency: "EUR", Amount: "10.0000"}},
			Inventory: InventoryView{Quantity: 5, Track: true},
			Suppliers: []VariantSupplierInput{{SupplierID: supplier.ID, SupplierSKU: "REF-FOURN-1"}},
		}},
	})
	if err != nil {
		t.Fatalf("create personalized product: %v", err)
	}
	if product.Brand == nil || product.Brand.ID != brand.ID || len(product.SupplierIDs) != 1 ||
		len(product.Variants[0].Suppliers) != 1 || product.Variants[0].Suppliers[0].SupplierSKU != "REF-FOURN-1" {
		t.Fatalf("catalog references were not returned: %+v", product)
	}
	if _, err := memory.UpdateSupplier(ctx, seed.ShopID, supplier.ID, SupplierInput{Code: supplier.Code, Name: supplier.Name, Active: false}); !errors.Is(err, ErrConflict) {
		t.Fatalf("deactivating an assigned supplier error = %v, want conflict", err)
	}

	tokenHash := sha256.Sum256([]byte("personalization cart"))
	if err := memory.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart: %v", err)
	}
	if _, err := memory.AddCartLine(ctx, seed.ShopID, tokenHash[:], product.Variants[0].ID, 1); !errors.Is(err, ErrInvalidPersonalization) {
		t.Fatalf("missing required selection error = %v, want invalid personalization", err)
	}
	for _, selection := range []string{"gift", "gift", "plain"} {
		if _, err := memory.AddCartLine(ctx, seed.ShopID, tokenHash[:], product.Variants[0].ID, 1, map[string]string{"emballage": selection}); err != nil {
			t.Fatalf("add customized variant %q: %v", selection, err)
		}
	}
	cart, err := memory.GetCart(ctx, seed.ShopID, tokenHash[:])
	if err != nil {
		t.Fatalf("read personalized cart: %v", err)
	}
	if len(cart.Items) != 2 {
		t.Fatalf("personalized cart lines = %d, want two distinct configurations", len(cart.Items))
	}
	for _, item := range cart.Items {
		if len(item.Personalizations) != 1 || item.Personalizations[0].Label != "Emballage cadeau" {
			t.Fatalf("cart personalization snapshot = %+v", item.Personalizations)
		}
	}

	memory.mu.Lock()
	memory.products[seed.ShopID+"/"+product.ID].PersonalizationOptions[0].Choices = []PersonalizationChoice{{Value: "plain", Label: "Sans papier cadeau"}}
	memory.mu.Unlock()
	details := CheckoutDetails{
		Email: "buyer@example.com", Name: "Client",
		ShippingCarrierCode: "colissimo",
		ShippingAddress:     ShippingAddress{Line1: "1 rue Test", City: "Paris", PostalCode: "75001", Country: "FR"},
	}
	if _, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], details); !errors.Is(err, ErrInvalidPersonalization) {
		t.Fatalf("checkout after choice removal error = %v, want invalid personalization", err)
	}
	memory.mu.Lock()
	memory.products[seed.ShopID+"/"+product.ID].PersonalizationOptions = clonePersonalizationOptions(options)
	memory.mu.Unlock()
	order, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], details)
	if err != nil {
		t.Fatalf("checkout personalized cart: %v", err)
	}
	if len(order.Items) != 2 || order.Subtotal != "30.0000" {
		t.Fatalf("customized order = %+v, want two lines and EUR 30.0000 subtotal", order)
	}
	values := make(map[string]int)
	for _, item := range order.Items {
		if len(item.Personalizations) != 1 {
			t.Fatalf("order personalization snapshot = %+v", item.Personalizations)
		}
		values[item.Personalizations[0].Value] = item.Quantity
	}
	if values["gift"] != 2 || values["plain"] != 1 {
		t.Fatalf("order personalization quantities = %v", values)
	}
}

func TestMemoryShippingWasmPolicyRequotesAndSnapshotsCheckout(t *testing.T) {
	ctx := context.Background()
	memory := NewMemory()
	memory.SetShippingPricingPolicy(testFreeShippingPolicy{amount: "0.0000"})
	tokenHash := sha256.Sum256([]byte("shipping policy cart"))
	if err := memory.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart: %v", err)
	}
	if _, err := memory.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 2); err != nil {
		t.Fatalf("add cart line: %v", err)
	}
	options, err := memory.ListCartShippingOptions(ctx, seed.ShopID, tokenHash[:], "FR")
	if err != nil || len(options) != 1 {
		t.Fatalf("shipping options = %+v, error = %v", options, err)
	}
	if options[0].Amount != "0.0000" || options[0].PricingSubtotal != "178.0000" ||
		options[0].PolicySnapshot == nil || options[0].PolicySnapshot.ModuleName != "test.shipping.policy" {
		t.Fatalf("shipping policy option = %+v", options[0])
	}
	order, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email: "shipping-policy@example.com", Name: "Shipping Policy",
		ShippingCarrierCode: "colissimo", ShippingServiceCode: "legacy",
		ExpectedShippingTotal: options[0].Amount, ExpectedShippingQuoteID: options[0].QuoteID,
		ShippingAddress: ShippingAddress{Line1: "1 rue Test", City: "Paris", PostalCode: "75001", Country: "FR"},
	})
	if err != nil {
		t.Fatalf("checkout cart: %v", err)
	}
	if order.ShippingTotal != "0.0000" || order.GrandTotal != "178.0000" ||
		order.ShippingPricingSubtotal == nil || *order.ShippingPricingSubtotal != "178.0000" ||
		order.ShippingPolicySnapshot == nil || order.ShippingPolicySnapshot.ModuleSHA256 != strings.Repeat("a", 64) {
		t.Fatalf("shipping policy order snapshot = %+v", order)
	}
}

func TestShippingPricingPolicyHostRejectsUnauthorizedProposal(t *testing.T) {
	_, _, err := ApplyShippingPricingPolicy(
		context.Background(), testFreeShippingPolicy{amount: "6.9000"}, "6.9000", "178.0000",
	)
	if !errors.Is(err, ErrShippingPolicyRejected) {
		t.Fatalf("unauthorized module amount error = %v, want %v", err, ErrShippingPolicyRejected)
	}
}

type testFreeShippingPolicy struct {
	amount string
}

func (p testFreeShippingPolicy) Evaluate(context.Context, string, string) (string, ShippingPricingPolicySnapshot, error) {
	return p.amount, ShippingPricingPolicySnapshot{
		ModuleName: "test.shipping.policy", ModuleVersion: "1.0.0",
		ModuleSHA256: strings.Repeat("a", 64), ContractVersion: ShippingPolicyContractVersion,
	}, nil
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
	memory.mu.Lock()
	memory.products[seed.ShopID+"/"+seed.ProductID].Variants[0].Inventory.StockStatus = "out_of_stock"
	memory.mu.Unlock()
	if _, err := memory.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 1); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("add unavailable product error = %v, want ErrOutOfStock", err)
	}
	if _, err := memory.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email:               "buyer@example.com",
		Name:                "Client de test",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	}); !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("checkout unavailable product error = %v, want ErrOutOfStock", err)
	}
	memory.mu.Lock()
	memory.products[seed.ShopID+"/"+seed.ProductID].Variants[0].Inventory.StockStatus = "in_stock"
	memory.mu.Unlock()

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
