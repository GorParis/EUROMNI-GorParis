//go:build integration

package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/testdb"
)

func TestPostgresStoreIntegration(t *testing.T) {
	ctx := context.Background()
	db := NewSQL(testdb.Start(t))
	shippingConfig, err := db.GetShippingConfig(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read default shipping configuration: %v", err)
	}
	carrierCode := "colissimo"
	for index := range shippingConfig.Carriers {
		if shippingConfig.Carriers[index].Code == carrierCode {
			shippingConfig.Carriers[index].Active = true
		}
	}
	shippingConfig.Rates = []ShippingRate{{
		CarrierCode: carrierCode,
		ZoneCode:    shippingConfig.Zones[0].Code,
		Amount:      "6.90",
		Currency:    "EUR",
		Active:      true,
	}}
	if err := db.ReplaceShippingConfig(ctx, seed.ShopID, *shippingConfig); err != nil {
		t.Fatalf("configure checkout shipping rate: %v", err)
	}

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
	if _, err := db.AuthenticateStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "integration-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("authenticate unconfigured seeded owner error = %v, want %v", err, ErrInvalidCredentials)
	}
	if err := db.BootstrapStaff(ctx, seed.ShopID, "wrong-owner@atelier-demo.example", "Wrong Owner", "integration-password"); !errors.Is(err, ErrStaffAlreadyBootstrapped) {
		t.Fatalf("bootstrap seeded owner with wrong email error = %v, want %v", err, ErrStaffAlreadyBootstrapped)
	}
	if err := db.BootstrapStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "Demo Owner", "integration-password"); err != nil {
		t.Fatalf("bootstrap seeded owner: %v", err)
	}
	if err := db.BootstrapStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "Demo Owner", "another-password"); !errors.Is(err, ErrStaffAlreadyBootstrapped) {
		t.Fatalf("second staff bootstrap error = %v, want %v", err, ErrStaffAlreadyBootstrapped)
	}
	staffSession, err := db.AuthenticateStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "integration-password")
	if err != nil {
		t.Fatalf("authenticate bootstrapped owner: %v", err)
	}
	if _, err := db.ValidateStaffSession(ctx, staffSession.Token); err != nil {
		t.Fatalf("validate staff session: %v", err)
	}
	if err := db.RevokeStaffSession(ctx, staffSession.Token); err != nil {
		t.Fatalf("revoke staff session: %v", err)
	}
	if _, err := db.ValidateStaffSession(ctx, staffSession.Token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked staff session error = %v, want %v", err, ErrInvalidCredentials)
	}
	expiredSession, err := db.AuthenticateStaff(ctx, seed.ShopID, "owner@atelier-demo.example", "integration-password")
	if err != nil {
		t.Fatalf("authenticate owner for expiry test: %v", err)
	}
	expiredHash := sha256.Sum256([]byte(expiredSession.Token))
	if _, err := db.SQL.ExecContext(ctx, `UPDATE sessions SET expires_at = now() - interval '1 second' WHERE token_hash = $1`, expiredHash[:]); err != nil {
		t.Fatalf("expire staff session: %v", err)
	}
	if _, err := db.ValidateStaffSession(ctx, expiredSession.Token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expired staff session error = %v, want %v", err, ErrInvalidCredentials)
	}
	if err := db.RevokeStaffSession(ctx, expiredSession.Token); err != nil {
		t.Fatalf("revoke expired staff session: %v", err)
	}

	emptyShopID := "e0e00000-0001-4000-8000-000000000099"
	if _, err := db.SQL.ExecContext(ctx, `INSERT INTO shops (id, slug, name) VALUES ($1, 'bootstrap-test', 'Bootstrap test')`, emptyShopID); err != nil {
		t.Fatalf("create empty bootstrap shop: %v", err)
	}
	if err := db.BootstrapStaff(ctx, emptyShopID, "new-owner@example.com", "New Owner", "empty-shop-password"); err != nil {
		t.Fatalf("bootstrap owner in empty shop: %v", err)
	}
	if err := db.BootstrapStaff(ctx, emptyShopID, "another-owner@example.com", "Another Owner", "another-password"); !errors.Is(err, ErrStaffAlreadyBootstrapped) {
		t.Fatalf("second empty-shop bootstrap error = %v, want %v", err, ErrStaffAlreadyBootstrapped)
	}
	emptyShopSession, err := db.AuthenticateStaff(ctx, emptyShopID, "new-owner@example.com", "empty-shop-password")
	if err != nil || emptyShopSession.Role != "owner" {
		t.Fatalf("authenticate empty-shop owner = %+v, error = %v", emptyShopSession, err)
	}
	staffMember, err := db.CreateStaffMember(ctx, seed.ShopID, StaffMemberInput{
		Email: "catalog-editor@example.com", Name: "Catalog Editor", Role: "editor", Password: "initial-editor-password",
	})
	if err != nil {
		t.Fatalf("create staff member: %v", err)
	}
	editorSession, err := db.AuthenticateStaff(ctx, seed.ShopID, staffMember.Email, "initial-editor-password")
	if err != nil || editorSession.Role != "editor" {
		t.Fatalf("authenticate staff member = %+v, error = %v", editorSession, err)
	}
	updatedStaff, err := db.UpdateStaffMember(ctx, seed.ShopID, staffMember.ID, StaffMemberInput{
		Email: staffMember.Email, Name: "Catalog Support", Role: "support", Password: "replacement-editor-password",
	})
	if err != nil || updatedStaff.Role != "support" || updatedStaff.Name != "Catalog Support" {
		t.Fatalf("update staff member = %+v, error = %v", updatedStaff, err)
	}
	if _, err := db.ValidateStaffSession(ctx, editorSession.Token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("validate session after password reset error = %v, want %v", err, ErrInvalidCredentials)
	}
	if _, err := db.AuthenticateStaff(ctx, seed.ShopID, staffMember.Email, "initial-editor-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("authenticate with old staff password error = %v, want %v", err, ErrInvalidCredentials)
	}
	supportSession, err := db.AuthenticateStaff(ctx, seed.ShopID, staffMember.Email, "replacement-editor-password")
	if err != nil || supportSession.Role != "support" {
		t.Fatalf("authenticate updated staff member = %+v, error = %v", supportSession, err)
	}
	if _, err := db.UpdateStaffMember(ctx, seed.ShopID, staffSession.UserID, StaffMemberInput{
		Email: staffSession.Email, Name: staffSession.Name, Role: "editor",
	}); !errors.Is(err, ErrStaffOwnerProtected) {
		t.Fatalf("change owner role error = %v, want %v", err, ErrStaffOwnerProtected)
	}
	if err := db.RemoveStaffMember(ctx, seed.ShopID, staffSession.UserID); !errors.Is(err, ErrStaffOwnerProtected) {
		t.Fatalf("remove owner error = %v, want %v", err, ErrStaffOwnerProtected)
	}
	if err := db.RemoveStaffMember(ctx, seed.ShopID, staffMember.ID); err != nil {
		t.Fatalf("remove staff member: %v", err)
	}
	if _, err := db.ValidateStaffSession(ctx, supportSession.Token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("validate removed staff session error = %v, want %v", err, ErrInvalidCredentials)
	}

	adminProduct, err := db.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug: "admin-crud-test", Status: "draft", CategoryIDs: []string{seed.CategoryID},
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article admin", Description: "Brouillon"}},
		Media: []MediaInput{
			{URI: "https://cdn.example.test/admin-main.webp", MimeType: "image/webp", Alt: "Image principale"},
			{URI: "https://cdn.example.test/admin-detail.webp", MimeType: "image/webp", Alt: "Détail"},
		},
		Variants: []VariantInput{{
			SKU: "ADMIN-CRUD-TEST", Options: map[string]any{"taille": "unique"},
			Prices:    []PriceView{{Currency: "EUR", Amount: "12.5000"}},
			Inventory: InventoryView{Quantity: 8, Track: true},
		}},
	})
	if err != nil {
		t.Fatalf("create admin product: %v", err)
	}
	if len(adminProduct.CategoryIDs) != 1 || adminProduct.CategoryIDs[0] != seed.CategoryID || len(adminProduct.Variants) != 1 ||
		adminProduct.Variants[0].SKU != "ADMIN-CRUD-TEST" || adminProduct.Variants[0].Prices[0].Amount != "12.5000" ||
		adminProduct.Variants[0].Inventory.Quantity != 8 || len(adminProduct.Media) != 2 ||
		adminProduct.Media[0].Position != 0 || adminProduct.Media[1].Position != 1 {
		t.Fatalf("created admin product associations and inventory = %+v", adminProduct)
	}
	primaryMediaID := adminProduct.Media[0].ID
	if _, err := db.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug: "cabas-lin", Status: "draft",
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Doublon"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate product error = %v, want %v", err, ErrConflict)
	}
	adminProducts, err := db.ListAdminProducts(ctx, seed.ShopID)
	if err != nil || len(adminProducts) != 6 {
		t.Fatalf("admin products = %d, error = %v; want 6 including draft", len(adminProducts), err)
	}
	updatedProduct, err := db.UpdateProduct(ctx, seed.ShopID, adminProduct.ID, ProductInput{
		Slug: "admin-crud-test", Status: "active", CategoryIDs: []string{seed.CategoryID},
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article publié", Description: "Mis à jour"}},
		Media: []MediaInput{
			{URI: "https://cdn.example.test/admin-main.webp", MimeType: "image/webp", Alt: "Image principale modifiée"},
		},
		Variants: []VariantInput{{
			SKU: "ADMIN-CRUD-TEST", Options: map[string]any{"taille": "unique"},
			Prices:    []PriceView{{Currency: "EUR", Amount: "15.0000"}},
			Inventory: InventoryView{Quantity: 6, Track: true},
		}},
	})
	if err != nil || updatedProduct.Status != "active" || updatedProduct.Translations[0].Name != "Article publié" {
		t.Fatalf("updated product = %+v, error = %v", updatedProduct, err)
	}
	if len(updatedProduct.CategoryIDs) != 1 || len(updatedProduct.Variants) != 1 ||
		updatedProduct.Variants[0].ID != adminProduct.Variants[0].ID ||
		updatedProduct.Variants[0].Prices[0].Amount != "15.0000" || updatedProduct.Variants[0].Inventory.Quantity != 6 ||
		len(updatedProduct.Media) != 1 || updatedProduct.Media[0].ID != primaryMediaID ||
		updatedProduct.Media[0].Position != 0 || updatedProduct.Media[0].Alt != "Image principale modifiée" {
		t.Fatalf("updated admin product associations and inventory = %+v", updatedProduct)
	}
	adminCartToken := sha256.Sum256([]byte("admin variant reference cart"))
	if err := db.CreateCart(ctx, seed.ShopID, adminCartToken[:]); err != nil {
		t.Fatalf("create admin variant reference cart: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", adminCartToken[:])
	}()
	if _, err := db.AddCartLine(ctx, seed.ShopID, adminCartToken[:], updatedProduct.Variants[0].ID, 1); err != nil {
		t.Fatalf("add admin variant to cart: %v", err)
	}
	if _, err := db.UpdateProduct(ctx, seed.ShopID, adminProduct.ID, ProductInput{
		Slug: "admin-crud-test", Status: "active",
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article publié"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("removing cart-referenced variant error = %v, want %v", err, ErrConflict)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, adminCartToken[:], CheckoutDetails{
		Email:               "admin-variant-order@example.com",
		Name:                "Admin variant order",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	}); err != nil {
		t.Fatalf("checkout cart containing admin variant: %v", err)
	}
	if _, err := db.UpdateProduct(ctx, seed.ShopID, adminProduct.ID, ProductInput{
		Slug: "admin-crud-test", Status: "active",
		Translations: []TranslationView{{Locale: "fr-FR", Name: "Article publié"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("removing order-referenced variant error = %v, want %v", err, ErrConflict)
	}
	if err := db.ArchiveProduct(ctx, seed.ShopID, adminProduct.ID); err != nil {
		t.Fatalf("archive admin product: %v", err)
	}
	if _, err := db.GetProduct(ctx, seed.ShopID, adminProduct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read archived product error = %v, want %v", err, ErrNotFound)
	}

	adminCategory, err := db.CreateCategory(ctx, seed.ShopID, CategoryInput{
		Slug:         "admin-crud-test",
		Translations: []CategoryTranslationView{{Locale: "fr-FR", Name: "Catégorie admin"}},
	})
	if err != nil {
		t.Fatalf("create admin category: %v", err)
	}
	if _, err := db.CreateCategory(ctx, seed.ShopID, CategoryInput{
		Slug: "sacs", Translations: []CategoryTranslationView{{Locale: "fr-FR", Name: "Doublon"}},
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate category error = %v, want %v", err, ErrConflict)
	}
	updatedCategory, err := db.UpdateCategory(ctx, seed.ShopID, adminCategory.ID, CategoryInput{
		Slug:         "admin-crud-modified",
		Translations: []CategoryTranslationView{{Locale: "fr-FR", Name: "Catégorie modifiée"}},
	})
	if err != nil || updatedCategory.Slug != "admin-crud-modified" {
		t.Fatalf("updated category = %+v, error = %v", updatedCategory, err)
	}
	if err := db.DeleteCategory(ctx, seed.ShopID, adminCategory.ID); err != nil {
		t.Fatalf("delete admin category: %v", err)
	}
	if _, err := db.GetCategory(ctx, seed.ShopID, adminCategory.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read deleted category error = %v, want %v", err, ErrNotFound)
	}
	var migrationCount int
	if err := db.SQL.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("read migration ledger: %v", err)
	}
	if migrationCount != 10 {
		t.Fatalf("recorded migrations = %d, want 10", migrationCount)
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
		Email:               "checkout@example.com",
		Name:                "Checkout test",
		ShippingCarrierCode: "colissimo",
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
	adminOrder, err := db.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil || adminOrder.Email != "checkout@example.com" || adminOrder.Name != "Checkout test" ||
		adminOrder.ShippingAddress.Country != "FR" || adminOrder.ShippingCarrierCode != "colissimo" ||
		adminOrder.ShippingCarrierName != "Colissimo" || adminOrder.ShippingZoneName != "France et Union européenne" ||
		adminOrder.ShippingTotal != "6.9000" || adminOrder.GrandTotal != "184.9000" ||
		len(adminOrder.Items) != 1 || adminOrder.Items[0].SKU != "CABAS-LIN-NAT" {
		t.Fatalf("read admin order = %+v, error = %v", adminOrder, err)
	}
	shippingConfig, err = db.GetShippingConfig(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read shipping config before tariff change: %v", err)
	}
	for index := range shippingConfig.Carriers {
		if shippingConfig.Carriers[index].Code == "colissimo" {
			shippingConfig.Carriers[index].Name = "Colissimo modifié"
		}
	}
	shippingConfig.Rates[0].Amount = "9.99"
	if err := db.ReplaceShippingConfig(ctx, seed.ShopID, *shippingConfig); err != nil {
		t.Fatalf("change shipping rate after order: %v", err)
	}
	snapshottedOrder, err := db.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil || snapshottedOrder.ShippingCarrierName != "Colissimo" ||
		snapshottedOrder.ShippingTotal != "6.9000" || snapshottedOrder.GrandTotal != "184.9000" {
		t.Fatalf("order shipping snapshot after tariff change = %+v, error = %v", snapshottedOrder, err)
	}
	adminOrderPage, err := db.ListAdminOrders(ctx, seed.ShopID, 1, 0)
	if err != nil || len(adminOrderPage) != 1 {
		t.Fatalf("first admin order page = %+v, error = %v; want one row", adminOrderPage, err)
	}
	adminOrders, err := db.ListAdminOrders(ctx, seed.ShopID, 100, 0)
	if err != nil {
		t.Fatalf("list admin orders: %v", err)
	}
	foundAdminOrder := false
	for _, listedOrder := range adminOrders {
		if listedOrder.ID == order.ID {
			foundAdminOrder = true
			break
		}
	}
	if !foundAdminOrder {
		t.Fatalf("admin order list %+v does not contain checkout order %s", adminOrders, order.ID)
	}
	emptyAdminOrderPage, err := db.ListAdminOrders(ctx, seed.ShopID, 10, 1000)
	if err != nil || len(emptyAdminOrderPage) != 0 {
		t.Fatalf("empty admin order page = %+v, error = %v; want no rows", emptyAdminOrderPage, err)
	}
	if _, err := db.GetAdminOrder(ctx, "e0e00000-0001-4000-8000-000000000099", order.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-shop admin order error = %v, want %v", err, ErrNotFound)
	}
	if err := db.CancelAdminOrder(ctx, seed.ShopID, order.ID); err != nil {
		t.Fatalf("cancel pending order: %v", err)
	}
	adminOrder, err = db.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil || adminOrder.Status != "cancelled" {
		t.Fatalf("cancelled order = %+v, error = %v", adminOrder, err)
	}
	if err := db.CancelAdminOrder(ctx, seed.ShopID, order.ID); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("cancel already-cancelled order error = %v, want %v", err, ErrOrderTransition)
	}
	if err := db.SQL.QueryRowContext(ctx, `SELECT quantity FROM inventory_items WHERE variant_id::text = $1`, seed.VariantID).Scan(&remainingStock); err != nil {
		t.Fatalf("read restocked inventory: %v", err)
	}
	if remainingStock != 24 {
		t.Fatalf("inventory after cancelling order = %d, want 24", remainingStock)
	}

	paymentCartToken := sha256.Sum256([]byte("payment protected order cart"))
	if err := db.CreateCart(ctx, seed.ShopID, paymentCartToken[:]); err != nil {
		t.Fatalf("create payment-protected order cart: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", paymentCartToken[:])
	}()
	if _, err := db.AddCartLine(ctx, seed.ShopID, paymentCartToken[:], seed.VariantID, 1); err != nil {
		t.Fatalf("add payment-protected order line: %v", err)
	}
	paymentOrder, err := db.CheckoutCart(ctx, seed.ShopID, paymentCartToken[:], CheckoutDetails{
		Email:               "payment-protected@example.com",
		Name:                "Payment protected",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("create payment-protected order: %v", err)
	}
	if _, err := db.SQL.ExecContext(ctx, `
		INSERT INTO payments (order_id, provider, amount, currency)
		VALUES ($1, 'test', 89, 'EUR')
	`, paymentOrder.ID); err != nil {
		t.Fatalf("add payment record: %v", err)
	}
	if err := db.CancelAdminOrder(ctx, seed.ShopID, paymentOrder.ID); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("cancel order with payment record error = %v, want %v", err, ErrOrderTransition)
	}

	fulfillmentCartToken := sha256.Sum256([]byte("manual payment fulfillment cart"))
	if err := db.CreateCart(ctx, seed.ShopID, fulfillmentCartToken[:]); err != nil {
		t.Fatalf("create manual-payment cart: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", fulfillmentCartToken[:])
	}()
	if _, err := db.AddCartLine(ctx, seed.ShopID, fulfillmentCartToken[:], seed.VariantID, 1); err != nil {
		t.Fatalf("add manual-payment cart line: %v", err)
	}
	fulfillmentOrder, err := db.CheckoutCart(ctx, seed.ShopID, fulfillmentCartToken[:], CheckoutDetails{
		Email:               "manual-payment@example.com",
		Name:                "Manual payment",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("create order for manual payment and fulfillment: %v", err)
	}
	if err := db.FulfillAdminOrder(ctx, seed.ShopID, fulfillmentOrder.ID, "TRACK-1"); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("fulfill unpaid order error = %v, want %v", err, ErrOrderTransition)
	}
	if err := db.ConfirmManualOrderPayment(ctx, seed.ShopID, fulfillmentOrder.ID, "BANK-TRANSFER-1"); err != nil {
		t.Fatalf("record verified manual payment: %v", err)
	}
	var paymentProvider, paymentStatus, paymentAmount, paymentCurrency, paymentReference string
	if err := db.SQL.QueryRowContext(ctx, `
		SELECT provider, status::text, amount::text, currency::text, provider_ref
		FROM payments WHERE order_id = $1
	`, fulfillmentOrder.ID).Scan(&paymentProvider, &paymentStatus, &paymentAmount, &paymentCurrency, &paymentReference); err != nil {
		t.Fatalf("read manually confirmed payment: %v", err)
	}
	if paymentProvider != "manual" || paymentStatus != "captured" ||
		paymentAmount != fulfillmentOrder.GrandTotal || paymentCurrency != fulfillmentOrder.Currency ||
		paymentReference != "BANK-TRANSFER-1" {
		t.Fatalf("manual payment snapshot = %s/%s %s %s ref=%q; want captured full-total payment", paymentProvider, paymentStatus, paymentAmount, paymentCurrency, paymentReference)
	}
	if err := db.ConfirmManualOrderPayment(ctx, seed.ShopID, fulfillmentOrder.ID, "BANK-TRANSFER-1"); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("record duplicate manual payment error = %v, want %v", err, ErrOrderTransition)
	}
	if err := db.CancelAdminOrder(ctx, seed.ShopID, fulfillmentOrder.ID); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("cancel manually paid order error = %v, want %v", err, ErrOrderTransition)
	}
	if err := db.FulfillAdminOrder(ctx, seed.ShopID, fulfillmentOrder.ID, "TRACK-1"); err != nil {
		t.Fatalf("fulfill paid order: %v", err)
	}
	if err := db.FulfillAdminOrder(ctx, seed.ShopID, fulfillmentOrder.ID, "TRACK-1"); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("fulfill order twice error = %v, want %v", err, ErrOrderTransition)
	}
	fulfilledOrder, err := db.GetAdminOrder(ctx, seed.ShopID, fulfillmentOrder.ID)
	if err != nil {
		t.Fatalf("read fulfilled order: %v", err)
	}
	if fulfilledOrder.Status != "fulfilled" || fulfilledOrder.ShipmentStatus != "shipped" || fulfilledOrder.Tracking != "TRACK-1" {
		t.Fatalf("fulfilled order snapshot = %+v", fulfilledOrder)
	}
}

func TestPostgresShippingConfiguration(t *testing.T) {
	ctx := context.Background()
	db := NewSQL(testdb.Start(t))
	config, err := db.GetShippingConfig(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded shipping configuration: %v", err)
	}
	if len(config.Carriers) != 4 || len(config.Zones) != 1 || len(config.Rates) != 0 {
		t.Fatalf("seeded shipping configuration = %+v, want four carriers, one zone and no invented rates", config)
	}

	updated := ShippingConfig{
		Carriers: []ShippingCarrier{
			{Code: "colissimo", Name: "Colissimo", Active: true},
			{Code: "custom-carrier", Name: "Transport personnalisé", Active: true},
		},
		Zones: []ShippingZone{{
			Code: "france", Name: "France métropolitaine", CountryCodes: []string{"FR"}, Active: true,
		}},
		Rates: []ShippingRate{{
			CarrierCode: "custom-carrier", ZoneCode: "france", Amount: "6.90", Currency: "EUR", Active: true,
		}},
	}
	if err := db.ReplaceShippingConfig(ctx, seed.ShopID, updated); err != nil {
		t.Fatalf("replace shipping configuration: %v", err)
	}
	saved, err := db.GetShippingConfig(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read replaced shipping configuration: %v", err)
	}
	if len(saved.Carriers) != 2 || len(saved.Zones) != 1 || len(saved.Rates) != 1 ||
		saved.Rates[0].CarrierCode != "custom-carrier" || saved.Rates[0].Amount != "6.9000" {
		t.Fatalf("saved shipping configuration = %+v", saved)
	}
	franceOptions, err := db.ListShippingOptions(ctx, seed.ShopID, "FR")
	if err != nil || len(franceOptions) != 1 || franceOptions[0].CarrierCode != "custom-carrier" ||
		franceOptions[0].Amount != "6.9000" {
		t.Fatalf("France shipping options = %+v, error = %v", franceOptions, err)
	}
	canadaOptions, err := db.ListShippingOptions(ctx, seed.ShopID, "CA")
	if err != nil || len(canadaOptions) != 0 {
		t.Fatalf("Canada shipping options = %+v, error = %v; want no options", canadaOptions, err)
	}
	rejectedToken := sha256.Sum256([]byte("unsupported destination checkout"))
	if err := db.CreateCart(ctx, seed.ShopID, rejectedToken[:]); err != nil {
		t.Fatalf("create cart for unsupported destination: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", rejectedToken[:])
	}()
	if _, err := db.AddCartLine(ctx, seed.ShopID, rejectedToken[:], seed.VariantID, 1); err != nil {
		t.Fatalf("add item for unsupported destination: %v", err)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, rejectedToken[:], CheckoutDetails{
		Email: "unsupported-destination@example.com", Name: "Unsupported destination",
		ShippingCarrierCode: "custom-carrier",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue du Test", City: "Toronto", PostalCode: "M5V 2T6", Country: "CA",
		},
	}); !errors.Is(err, ErrShippingUnavailable) {
		t.Fatalf("checkout with unsupported destination error = %v, want %v", err, ErrShippingUnavailable)
	}

	weightedToken := sha256.Sum256([]byte("weight-value shipping quote"))
	weightedEmail := "weight-value-shipping@example.com"
	var weightedOrderID string
	t.Cleanup(func() {
		if weightedOrderID != "" {
			if _, err := db.SQL.ExecContext(context.Background(), `DELETE FROM orders WHERE id = $1`, weightedOrderID); err != nil {
				t.Errorf("clean up weight-value order: %v", err)
			}
		}
		if _, err := db.SQL.ExecContext(context.Background(), `DELETE FROM carts WHERE access_token_hash = $1`, weightedToken[:]); err != nil {
			t.Errorf("clean up weight-value cart: %v", err)
		}
		if _, err := db.SQL.ExecContext(context.Background(), `DELETE FROM customers WHERE shop_id = $1 AND email = $2`, seed.ShopID, weightedEmail); err != nil {
			t.Errorf("clean up weight-value customer: %v", err)
		}
	})
	if err := db.CreateCart(ctx, seed.ShopID, weightedToken[:]); err != nil {
		t.Fatalf("create weight-value cart: %v", err)
	}
	if _, err := db.AddCartLine(ctx, seed.ShopID, weightedToken[:], seed.VariantID, 1); err != nil {
		t.Fatalf("add weight-value cart line: %v", err)
	}
	cart, err := db.GetCart(ctx, seed.ShopID, weightedToken[:])
	if err != nil {
		t.Fatalf("read weight-value cart: %v", err)
	}
	subtotalBoundary := cart.Total.Amount
	weightedConfig := updated
	weightedConfig.Rates = []ShippingRate{{
		CarrierCode: "custom-carrier", ZoneCode: "france", Mode: "weight_value",
		Currency: "EUR", Active: true,
		Rules: []ShippingRateRule{
			{MinWeightGrams: 0, MaxWeightGrams: storeInt64Pointer(1000), MinSubtotal: "0", MaxSubtotal: &subtotalBoundary, Amount: "5.00"},
			{MinWeightGrams: 0, MaxWeightGrams: storeInt64Pointer(1000), MinSubtotal: subtotalBoundary, Amount: "4.00"},
			{MinWeightGrams: 1000, MinSubtotal: "0", MaxSubtotal: &subtotalBoundary, Amount: "9.00"},
			{MinWeightGrams: 1000, MinSubtotal: subtotalBoundary, Amount: "7.00"},
		},
	}}
	if err := db.ReplaceShippingConfig(ctx, seed.ShopID, weightedConfig); err != nil {
		t.Fatalf("configure weight-value shipping rates: %v", err)
	}

	if _, err := db.SQL.ExecContext(ctx, `UPDATE variants SET weight_grams = 500 WHERE id = $1`, seed.VariantID); err != nil {
		t.Fatalf("set test variant weight: %v", err)
	}
	options, err := db.ListCartShippingOptions(ctx, seed.ShopID, weightedToken[:], "FR")
	if err != nil || len(options) != 1 || options[0].Amount != "4.0000" ||
		options[0].WeightGrams == nil || *options[0].WeightGrams != 500 {
		t.Fatalf("500g quote at subtotal upper boundary = %+v, error = %v", options, err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE variants SET weight_grams = 1000 WHERE id = $1`, seed.VariantID); err != nil {
		t.Fatalf("set test variant boundary weight: %v", err)
	}
	options, err = db.ListCartShippingOptions(ctx, seed.ShopID, weightedToken[:], "FR")
	if err != nil || len(options) != 1 || options[0].Amount != "7.0000" {
		t.Fatalf("1000g quote at inclusive weight boundary = %+v, error = %v", options, err)
	}
	initialQuoteID := options[0].QuoteID
	if initialQuoteID == "" {
		t.Fatal("weight-value quote has no quote id")
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE variants SET weight_grams = NULL WHERE id = $1`, seed.VariantID); err != nil {
		t.Fatalf("remove test variant weight: %v", err)
	}
	options, err = db.ListCartShippingOptions(ctx, seed.ShopID, weightedToken[:], "FR")
	if err != nil || len(options) != 0 {
		t.Fatalf("quote with missing variant weight = %+v, error = %v; want no fallback", options, err)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, weightedToken[:], CheckoutDetails{
		Email: weightedEmail, Name: "Weight Value Shipping",
		ShippingCarrierCode: "custom-carrier", ExpectedShippingTotal: "7.00",
		ShippingAddress: ShippingAddress{Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR"},
	}); !errors.Is(err, ErrShippingUnavailable) {
		t.Fatalf("checkout with missing weight error = %v, want unavailable shipping", err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE variants SET weight_grams = 1000 WHERE id = $1`, seed.VariantID); err != nil {
		t.Fatalf("restore test variant weight: %v", err)
	}
	weightedConfig.Rates[0].Rules[3].Amount = "8.00"
	if err := db.ReplaceShippingConfig(ctx, seed.ShopID, weightedConfig); err != nil {
		t.Fatalf("change weight-value shipping price: %v", err)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, weightedToken[:], CheckoutDetails{
		Email: weightedEmail, Name: "Weight Value Shipping",
		ShippingCarrierCode: "custom-carrier", ExpectedShippingTotal: "7.00",
		ExpectedShippingQuoteID: initialQuoteID,
		ShippingAddress: ShippingAddress{Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR"},
	}); !errors.Is(err, ErrShippingQuoteChanged) {
		t.Fatalf("checkout with stale quote error = %v, want quote changed", err)
	}
	options, err = db.ListCartShippingOptions(ctx, seed.ShopID, weightedToken[:], "FR")
	if err != nil || len(options) != 1 || options[0].Amount != "8.0000" {
		t.Fatalf("refreshed 1000g quote = %+v, error = %v", options, err)
	}
	priceQuoteID := options[0].QuoteID
	changedMaxWeight := int64(2000)
	weightedConfig.Rates[0].Rules[3].MaxWeightGrams = &changedMaxWeight
	if err := db.ReplaceShippingConfig(ctx, seed.ShopID, weightedConfig); err != nil {
		t.Fatalf("change weight-value rule bounds without changing price: %v", err)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, weightedToken[:], CheckoutDetails{
		Email: weightedEmail, Name: "Weight Value Shipping",
		ShippingCarrierCode: "custom-carrier", ExpectedShippingTotal: "8.00",
		ExpectedShippingQuoteID: priceQuoteID,
		ShippingAddress: ShippingAddress{Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR"},
	}); !errors.Is(err, ErrShippingQuoteChanged) {
		t.Fatalf("checkout with stale same-price quote error = %v, want quote changed", err)
	}
	options, err = db.ListCartShippingOptions(ctx, seed.ShopID, weightedToken[:], "FR")
	if err != nil || len(options) != 1 || options[0].Amount != "8.0000" || options[0].QuoteID == priceQuoteID {
		t.Fatalf("same-price refreshed quote = %+v, error = %v", options, err)
	}
	order, err := db.CheckoutCart(ctx, seed.ShopID, weightedToken[:], CheckoutDetails{
		Email: weightedEmail, Name: "Weight Value Shipping",
		ShippingCarrierCode: "custom-carrier", ExpectedShippingTotal: options[0].Amount,
		ExpectedShippingQuoteID: options[0].QuoteID,
		ShippingAddress: ShippingAddress{Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR"},
	})
	if err != nil {
		t.Fatalf("checkout with current weight-value quote: %v", err)
	}
	weightedOrderID = order.ID
	if order.ShippingRateMode != "weight_value" || order.ShippingWeightGrams == nil ||
		*order.ShippingWeightGrams != 1000 || order.ShippingPricingSubtotal == nil ||
		*order.ShippingPricingSubtotal != subtotalBoundary || len(order.ShippingRateRule) == 0 {
		t.Fatalf("weight-value checkout snapshot = %+v", order)
	}
	adminSnapshot, err := db.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil || adminSnapshot.ShippingRateMode != "weight_value" ||
		adminSnapshot.ShippingWeightGrams == nil || *adminSnapshot.ShippingWeightGrams != 1000 {
		t.Fatalf("weight-value admin snapshot = %+v, error = %v", adminSnapshot, err)
	}

	otherShop, err := db.GetShippingConfig(ctx, "e0e00000-0001-4000-8000-000000000099")
	if err != nil {
		t.Fatalf("read other shop shipping configuration: %v", err)
	}
	if len(otherShop.Carriers) != 0 || len(otherShop.Zones) != 0 || len(otherShop.Rates) != 0 {
		t.Fatalf("other shop received shipping configuration: %+v", otherShop)
	}
}

func storeInt64Pointer(value int64) *int64 { return &value }
