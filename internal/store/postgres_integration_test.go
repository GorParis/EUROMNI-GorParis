//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/testdb"
)

func TestPostgresStoreIntegration(t *testing.T) {
	ctx := context.Background()
	db := NewSQL(testdb.Start(t))

	product, err := db.GetProduct(ctx, seed.ShopID, seed.ProductID)
	if err != nil {
		t.Fatalf("read seeded product: %v", err)
	}
	if product.Slug != "cabas-lin" || len(product.Variants) != 1 {
		t.Fatalf("unexpected seeded product: %+v", product)
	}

	products, err := db.ListProducts(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("list seeded products: %v", err)
	}
	if len(products) != 5 || products[0].Slug != "bougie-cedre" || products[2].Slug != "ceinture-cuir" || len(products[2].Variants) != 3 {
		t.Fatalf("unexpected catalog list: %+v", products)
	}
	if len(products[0].Variants) != 1 || len(products[0].Variants[0].Options) != 0 || len(products[1].Variants) != 1 || len(products[3].Variants) != 3 || len(products[4].Variants) != 1 || len(products[4].Variants[0].Options) != 0 {
		t.Fatalf("unexpected mix of single-SKU and variant products: %+v", products)
	}

	categories, err := db.ListCategories(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded categories: %v", err)
	}
	if len(categories) != 1 || categories[0].ID != seed.CategoryID || categories[0].Slug != "sacs" {
		t.Fatalf("unexpected seeded categories: %+v", categories)
	}

	category, err := db.GetCategory(ctx, seed.ShopID, seed.CategoryID)
	if err != nil {
		t.Fatalf("read seeded category: %v", err)
	}
	if len(category.Translations) != 2 {
		t.Fatalf("category translations = %d, want 2", len(category.Translations))
	}
	var migrationCount int
	if err := db.SQL.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("read migration ledger: %v", err)
	}
	if migrationCount != 6 {
		t.Fatalf("recorded migrations = %d, want 6", migrationCount)
	}
	installedModule, err := db.GetInstalledModule(ctx, seed.ShopID, seed.ModuleID)
	if err != nil {
		t.Fatalf("read installed module: %v", err)
	}
	if installedModule.Name != "demo.loyalty" || installedModule.ABIVersion != "euromni.abi.v1" || installedModule.WASMSHA256 != nil {
		t.Fatalf("installed module = %+v, want demo module without registered artifact hash", installedModule)
	}
	expectedCapabilities := map[string]bool{"catalog": false, "customer": false, "hooks": false, "module.kv": false}
	for _, capability := range installedModule.Capabilities {
		if _, exists := expectedCapabilities[capability]; exists {
			expectedCapabilities[capability] = true
		}
	}
	for capability, found := range expectedCapabilities {
		if !found {
			t.Errorf("installed module missing capability %q: %v", capability, installedModule.Capabilities)
		}
	}

	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		t.Fatal("generate cart token failed")
	}
	tokenHash := sha256.Sum256(rawToken)
	if err := db.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", tokenHash[:])
	}()

	cart, err := db.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 2)
	if err != nil {
		t.Fatalf("add seeded variant: %v", err)
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Fatalf("cart items = %+v, want one line with quantity 2", cart.Items)
	}
	if cart.Total.Amount != "178.0000" || cart.Total.Currency != "EUR" {
		t.Fatalf("cart total = %+v, want EUR 178.0000", cart.Total)
	}
	if _, err := db.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 23); err != ErrOutOfStock {
		t.Fatalf("overstock error = %v, want %v", err, ErrOutOfStock)
	}

	order, err := db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email: "checkout@example.com",
		Name:  "Checkout test",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout cart: %v", err)
	}
	if order.Status != "pending" || order.Subtotal != "178.0000" || len(order.Items) != 1 || order.Items[0].SKU != "CABAS-LIN-NAT" {
		t.Fatalf("unexpected order result: %+v", order)
	}
	var remainingStock, savedLineCount, savedLineQuantity, savedOrders int
	if err := db.SQL.QueryRowContext(ctx, `SELECT quantity FROM inventory_items WHERE variant_id::text = $1`, seed.VariantID).Scan(&remainingStock); err != nil {
		t.Fatalf("read remaining stock: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*), COALESCE(sum(quantity), 0)::int FROM order_lines WHERE order_id::text = $1 AND name = 'Cabas en lin' AND unit_price = 89 AND line_total = 178`, order.ID).Scan(&savedLineCount, &savedLineQuantity); err != nil {
		t.Fatalf("read order line snapshot: %v", err)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM orders WHERE id::text = $1 AND shipping_address->>'country' = 'FR'`, order.ID).Scan(&savedOrders); err != nil {
		t.Fatalf("read saved order: %v", err)
	}
	if remainingStock != 22 || savedLineCount != 1 || savedLineQuantity != 2 || savedOrders != 1 {
		t.Fatalf("checkout persistence stock/orders/lines/quantity = %d/%d/%d/%d, want 22/1/1/2", remainingStock, savedOrders, savedLineCount, savedLineQuantity)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{}); err != ErrEmptyCart {
		t.Fatalf("second checkout error = %v, want %v", err, ErrEmptyCart)
	}
}
