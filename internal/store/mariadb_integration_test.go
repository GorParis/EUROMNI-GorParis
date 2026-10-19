//go:build integration

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"time"

	"euromni/internal/dbdialect"
	"euromni/internal/migrations"
	"euromni/internal/seed"
	schemafiles "euromni/migrations"
)

func TestMariaDBMigrationAndCommerceFlow(t *testing.T) {
	dsn := os.Getenv("MARIADB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MARIADB_TEST_DATABASE_URL must point to a dedicated MariaDB test database")
	}
	driverName, driverDSN, dialect, err := dbdialect.Resolve(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if dialect != dbdialect.MariaDB {
		t.Fatalf("MARIADB_TEST_DATABASE_URL resolved to %q, want MariaDB", dialect)
	}
	database, err := sql.Open(driverName, driverDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close MariaDB test database: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	source, err := fs.Sub(schemafiles.MariaDBFiles, "mariadb")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrations.ApplyMariaDB(ctx, database, source); err != nil {
		t.Fatalf("apply MariaDB migrations: %v", err)
	}

	store := NewSQLWithDialect(database, dbdialect.MariaDB)
	shippingConfig, err := store.GetShippingConfig(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded MariaDB shipping config: %v", err)
	}
	for index := range shippingConfig.Carriers {
		if shippingConfig.Carriers[index].Code == "colissimo" {
			shippingConfig.Carriers[index].Active = true
		}
	}
	shippingConfig.Services = []ShippingService{{
		CarrierCode: "colissimo", Code: "legacy", Name: "Offre historique", Active: true,
	}}
	shippingConfig.Rates = []ShippingRate{{
		CarrierCode: "colissimo", ServiceCode: "legacy", ZoneCode: "france-ue", Mode: "fixed",
		Amount: "6.90", Currency: "EUR", Active: true,
	}}
	if err := store.ReplaceShippingConfig(ctx, seed.ShopID, *shippingConfig); err != nil {
		t.Fatalf("reset MariaDB integration shipping config: %v", err)
	}
	products, err := store.ListProducts(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded products: %v", err)
	}
	if len(products) < 5 {
		t.Fatalf("MariaDB seed created %d storefront products, want at least 5", len(products))
	}
	foundCabas := false
	for _, product := range products {
		if product.Slug == "cabas-lin" {
			foundCabas = true
			break
		}
	}
	if !foundCabas {
		t.Fatal("MariaDB seed is missing the cabas-lin storefront product")
	}
	categories, err := store.ListCategories(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read seeded categories: %v", err)
	}
	foundSacs := len(categories) == 1 && categories[0].Slug == "sacs"
	if foundSacs {
		foundSacs = false
		for _, translation := range categories[0].Translations {
			if translation.Locale == "fr-FR" && translation.Name == "Sacs" {
				foundSacs = true
				break
			}
		}
	}
	if !foundSacs {
		t.Fatalf("unexpected MariaDB demo categories: %#v", categories)
	}
	module, err := store.GetInstalledModule(ctx, seed.ShopID, seed.ModuleID)
	if err != nil {
		t.Fatalf("read seeded module: %v", err)
	}
	if module.Name != "demo.loyalty" || len(module.Capabilities) != 4 {
		t.Fatalf("unexpected MariaDB demo module: %#v", module)
	}
	options, err := store.ListShippingOptions(ctx, seed.ShopID, "FR")
	if err != nil {
		t.Fatalf("read seeded shipping options: %v", err)
	}
	if len(options) == 0 || options[0].CarrierCode != "colissimo" ||
		options[0].ZoneCode != "france-ue" || options[0].ZoneName != "France et Union européenne" {
		t.Fatalf("unexpected seeded shipping options: %#v", options)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	product, err := store.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug:   "mariadb-test-" + suffix,
		Status: "active",
		Translations: []TranslationView{{
			Locale: "fr-FR", Name: "Produit MariaDB", Description: "Test d'intégration",
		}},
		Variants: []VariantInput{{
			SKU:         "MDB-" + suffix,
			Options:     map[string]any{"taille": "unique"},
			WeightGrams: mariaDBInt64Pointer(750),
			Prices:      []PriceView{{Currency: "EUR", Amount: "12.5000"}},
			Inventory:   InventoryView{Quantity: 5, Track: true},
		}},
	})
	if err != nil {
		t.Fatalf("create MariaDB product: %v", err)
	}
	tokenHash := sha256.Sum256([]byte("mariadb-test-cart-" + suffix))
	email := "mariadb-test-" + suffix + "@example.invalid"
	weightedTokenHash := sha256.Sum256([]byte("mariadb-weighted-cart-" + suffix))
	weightedEmail := "mariadb-weighted-" + suffix + "@example.invalid"
	personalizedTokenHash := sha256.Sum256([]byte("mariadb-personalized-cart-" + suffix))
	personalizedEmail := "mariadb-personalized-" + suffix + "@example.invalid"
	var orderID string
	var weightedOrderID string
	var personalizedOrderID string
	var paymentOrderIDs []string
	var paymentCartHashes [][32]byte
	var paymentEmails []string
	var personalizedProductID string
	var personalizedBrandID string
	var personalizedSupplierID string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if orderID != "" {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM shipments WHERE order_id = $1`, orderID); err != nil {
				t.Errorf("clean up MariaDB test shipments: %v", err)
			}
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM payments WHERE order_id = $1`, orderID); err != nil {
				t.Errorf("clean up MariaDB test payments: %v", err)
			}
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM orders WHERE id = $1`, orderID); err != nil {
				t.Errorf("clean up MariaDB test order: %v", err)
			}
		}
		if weightedOrderID != "" {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM orders WHERE id = $1`, weightedOrderID); err != nil {
				t.Errorf("clean up MariaDB weight-value order: %v", err)
			}
		}
		if personalizedOrderID != "" {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM orders WHERE id = $1`, personalizedOrderID); err != nil {
				t.Errorf("clean up MariaDB personalized order: %v", err)
			}
		}
		for _, paymentOrderID := range paymentOrderIDs {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM shipments WHERE order_id = $1`, paymentOrderID); err != nil {
				t.Errorf("clean up MariaDB development payment shipment: %v", err)
			}
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM payments WHERE order_id = $1`, paymentOrderID); err != nil {
				t.Errorf("clean up MariaDB development payment: %v", err)
			}
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM orders WHERE id = $1`, paymentOrderID); err != nil {
				t.Errorf("clean up MariaDB development payment order: %v", err)
			}
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM carts WHERE access_token_hash = $1`, tokenHash[:]); err != nil {
			t.Errorf("clean up MariaDB test cart: %v", err)
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM carts WHERE access_token_hash = $1`, weightedTokenHash[:]); err != nil {
			t.Errorf("clean up MariaDB weight-value cart: %v", err)
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM carts WHERE access_token_hash = $1`, personalizedTokenHash[:]); err != nil {
			t.Errorf("clean up MariaDB personalized cart: %v", err)
		}
		for _, paymentCartHash := range paymentCartHashes {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM carts WHERE access_token_hash = $1`, paymentCartHash[:]); err != nil {
				t.Errorf("clean up MariaDB development payment cart: %v", err)
			}
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM customers WHERE shop_id = $1 AND email = $2`, seed.ShopID, email); err != nil {
			t.Errorf("clean up MariaDB test customer: %v", err)
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM customers WHERE shop_id = $1 AND email = $2`, seed.ShopID, weightedEmail); err != nil {
			t.Errorf("clean up MariaDB weight-value customer: %v", err)
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM customers WHERE shop_id = $1 AND email = $2`, seed.ShopID, personalizedEmail); err != nil {
			t.Errorf("clean up MariaDB personalized customer: %v", err)
		}
		for _, paymentEmail := range paymentEmails {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM customers WHERE shop_id = $1 AND email = $2`, seed.ShopID, paymentEmail); err != nil {
				t.Errorf("clean up MariaDB development payment customer: %v", err)
			}
		}
		if personalizedProductID != "" {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM products WHERE id = $1`, personalizedProductID); err != nil {
				t.Errorf("clean up MariaDB personalized product: %v", err)
			}
		}
		if personalizedBrandID != "" {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM brands WHERE id = $1`, personalizedBrandID); err != nil {
				t.Errorf("clean up MariaDB integration brand: %v", err)
			}
		}
		if personalizedSupplierID != "" {
			if _, err := database.ExecContext(cleanupCtx, `DELETE FROM suppliers WHERE id = $1`, personalizedSupplierID); err != nil {
				t.Errorf("clean up MariaDB integration supplier: %v", err)
			}
		}
		if _, err := database.ExecContext(cleanupCtx, `DELETE FROM products WHERE id = $1`, product.ID); err != nil {
			t.Errorf("clean up MariaDB test product: %v", err)
		}
	})
	if err := store.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create MariaDB cart: %v", err)
	}
	if _, err := store.AddCartLine(ctx, seed.ShopID, tokenHash[:], product.Variants[0].ID, 1); err != nil {
		t.Fatalf("add MariaDB cart line: %v", err)
	}
	store.SetShippingPricingPolicy(testFreeShippingPolicy{amount: "6.9000"})
	t.Cleanup(func() {
		store.SetShippingPricingPolicy(nil)
	})
	order, err := store.CheckoutCart(ctx, seed.ShopID, tokenHash[:], CheckoutDetails{
		Email:               email,
		Name:                "MariaDB Integration",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: ShippingAddress{
			Line1: "1 rue de test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout MariaDB cart: %v", err)
	}
	orderID = order.ID
	adminOrder, err := store.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil {
		t.Fatalf("read MariaDB admin order: %v", err)
	}
	if adminOrder.Email != email || adminOrder.ShippingCarrierCode != "colissimo" ||
		adminOrder.ShippingPolicySnapshot == nil ||
		adminOrder.ShippingPolicySnapshot.ModuleName != "test.shipping.policy" {
		t.Fatalf("unexpected MariaDB order snapshot: %#v", adminOrder)
	}
	if err := store.ConfirmManualOrderPayment(ctx, seed.ShopID, order.ID, "MDB-TRANSFER"); err != nil {
		t.Fatalf("record MariaDB manual payment: %v", err)
	}
	var paymentProvider, paymentStatus, paymentAmount, paymentCurrency, paymentReference string
	if err := database.QueryRowContext(ctx, `
		SELECT provider, status, amount, currency, provider_ref
		FROM payments WHERE order_id = $1
	`, order.ID).Scan(&paymentProvider, &paymentStatus, &paymentAmount, &paymentCurrency, &paymentReference); err != nil {
		t.Fatalf("read MariaDB manual payment: %v", err)
	}
	if paymentProvider != "manual" || paymentStatus != "captured" ||
		paymentAmount != order.GrandTotal || paymentCurrency != order.Currency || paymentReference != "MDB-TRANSFER" {
		t.Fatalf("unexpected MariaDB payment snapshot: %s/%s %s %s ref=%q", paymentProvider, paymentStatus, paymentAmount, paymentCurrency, paymentReference)
	}
	if err := store.CancelAdminOrder(ctx, seed.ShopID, order.ID); !errors.Is(err, ErrOrderTransition) {
		t.Fatalf("cancel paid MariaDB test order error = %v, want %v", err, ErrOrderTransition)
	}
	if err := store.FulfillAdminOrder(ctx, seed.ShopID, order.ID, "MDB-TRACK-1"); err != nil {
		t.Fatalf("fulfill MariaDB test order: %v", err)
	}
	adminOrder, err = store.GetAdminOrder(ctx, seed.ShopID, order.ID)
	if err != nil {
		t.Fatalf("read fulfilled MariaDB order: %v", err)
	}
	if adminOrder.Status != "fulfilled" || adminOrder.ShipmentStatus != "shipped" || adminOrder.Tracking != "MDB-TRACK-1" {
		t.Fatalf("unexpected fulfilled MariaDB order: %#v", adminOrder)
	}

	for _, test := range []struct {
		outcome       string
		paymentStatus string
		orderStatus   string
		fulfill       bool
	}{
		{outcome: "accepted", paymentStatus: "captured", orderStatus: "paid", fulfill: true},
		{outcome: "declined", paymentStatus: "failed", orderStatus: "cancelled"},
		{outcome: "abandoned", paymentStatus: "cancelled", orderStatus: "cancelled"},
	} {
		t.Run("development-payment-"+test.outcome, func(t *testing.T) {
			paymentCartHash := sha256.Sum256([]byte("mariadb-payment-cart-" + suffix + "-" + test.outcome))
			paymentEmail := "mariadb-payment-" + suffix + "-" + test.outcome + "@example.invalid"
			paymentCartHashes = append(paymentCartHashes, paymentCartHash)
			paymentEmails = append(paymentEmails, paymentEmail)
			if err := store.CreateCart(ctx, seed.ShopID, paymentCartHash[:]); err != nil {
				t.Fatalf("create MariaDB payment cart: %v", err)
			}
			if _, err := store.AddCartLine(ctx, seed.ShopID, paymentCartHash[:], product.Variants[0].ID, 1); err != nil {
				t.Fatalf("add MariaDB payment cart line: %v", err)
			}
			var stockBefore int
			if err := database.QueryRowContext(ctx, `
				SELECT quantity FROM inventory_items WHERE variant_id = $1
			`, product.Variants[0].ID).Scan(&stockBefore); err != nil {
				t.Fatalf("read MariaDB stock before payment checkout: %v", err)
			}
			paymentOrder, err := store.CheckoutCart(ctx, seed.ShopID, paymentCartHash[:], CheckoutDetails{
				Email: paymentEmail, Name: "MariaDB Development Payment",
				ShippingCarrierCode: "colissimo", ShippingServiceCode: "legacy",
				ShippingAddress: ShippingAddress{
					Line1: "1 rue de test", City: "Paris", PostalCode: "75001", Country: "FR",
				},
			})
			if err != nil {
				t.Fatalf("checkout MariaDB payment cart: %v", err)
			}
			paymentOrderIDs = append(paymentOrderIDs, paymentOrder.ID)
			attempt, err := store.CreateDevelopmentPaymentAttempt(ctx, seed.ShopID, paymentOrder.ID)
			if err != nil {
				t.Fatalf("create MariaDB development payment attempt: %v", err)
			}
			if _, err := store.CreateDevelopmentPaymentAttempt(ctx, seed.ShopID, paymentOrder.ID); !errors.Is(err, ErrOrderTransition) {
				t.Fatalf("create duplicate MariaDB payment attempt error = %v, want %v", err, ErrOrderTransition)
			}
			result := PaymentResult{
				PaymentID: attempt.ID, OrderID: paymentOrder.ID, Provider: attempt.Provider,
				Amount: attempt.Amount, Currency: attempt.Currency, Outcome: test.outcome,
				EventID: "mariadb-event-" + attempt.ID, Reference: "mariadb-ref-" + attempt.ID,
			}
			tampered := result
			tampered.Currency = "USD"
			if err := store.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, tampered); !errors.Is(err, ErrInvalidPaymentResult) {
				t.Fatalf("apply MariaDB payment with wrong currency error = %v, want %v", err, ErrInvalidPaymentResult)
			}
			tampered = result
			tampered.Amount = "0.0100"
			if err := store.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, tampered); !errors.Is(err, ErrInvalidPaymentResult) {
				t.Fatalf("apply MariaDB payment with wrong amount error = %v, want %v", err, ErrInvalidPaymentResult)
			}
			if err := store.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, result); err != nil {
				t.Fatalf("apply MariaDB %s payment result: %v", test.outcome, err)
			}
			if err := store.ApplyDevelopmentPaymentResult(ctx, seed.ShopID, result); err != nil {
				t.Fatalf("replay MariaDB %s payment result: %v", test.outcome, err)
			}
			var savedPaymentStatus, savedOrderStatus string
			if err := database.QueryRowContext(ctx, `
				SELECT payment.status, orders.status
				FROM payments payment JOIN orders ON orders.id = payment.order_id
				WHERE payment.id = $1
			`, attempt.ID).Scan(&savedPaymentStatus, &savedOrderStatus); err != nil {
				t.Fatalf("read MariaDB development payment result: %v", err)
			}
			if savedPaymentStatus != test.paymentStatus || savedOrderStatus != test.orderStatus {
				t.Fatalf("MariaDB payment/order state = %s/%s; want %s/%s",
					savedPaymentStatus, savedOrderStatus, test.paymentStatus, test.orderStatus)
			}
			var stockAfter int
			if err := database.QueryRowContext(ctx, `
				SELECT quantity FROM inventory_items WHERE variant_id = $1
			`, product.Variants[0].ID).Scan(&stockAfter); err != nil {
				t.Fatalf("read MariaDB stock after payment: %v", err)
			}
			wantStock := stockBefore - 1
			if !test.fulfill {
				wantStock = stockBefore
			}
			if stockAfter != wantStock {
				t.Fatalf("MariaDB stock after %s result/replay = %d, want %d", test.outcome, stockAfter, wantStock)
			}
			if test.fulfill {
				if err := store.FulfillAdminOrder(ctx, seed.ShopID, paymentOrder.ID, "MDB-DEV-TRACK"); err != nil {
					t.Fatalf("fulfill accepted MariaDB payment order: %v", err)
				}
			} else if err := store.FulfillAdminOrder(ctx, seed.ShopID, paymentOrder.ID, ""); !errors.Is(err, ErrOrderTransition) {
				t.Fatalf("fulfill unpaid MariaDB order error = %v, want %v", err, ErrOrderTransition)
			}
		})
	}

	store.SetShippingPricingPolicy(nil)
	config, err := store.GetShippingConfig(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read MariaDB shipping config before weight-value test: %v", err)
	}
	for index := range config.Carriers {
		if config.Carriers[index].Code == "colissimo" {
			config.Carriers[index].Active = true
		}
	}
	config.Services = append(config.Services, ShippingService{
		CarrierCode: "colissimo", Code: "home", Name: "Domicile", Active: true,
	})
	config.Rates = []ShippingRate{{
		CarrierCode: "colissimo", ServiceCode: "home", ZoneCode: "france-ue", Mode: "weight_value",
		Currency: "EUR", Active: true,
		Rules: []ShippingRateRule{{
			MinWeightGrams: 0, MinSubtotal: "0", Amount: "4.50",
		}},
	}}
	if err := store.ReplaceShippingConfig(ctx, seed.ShopID, *config); err != nil {
		t.Fatalf("configure MariaDB weight-value rate: %v", err)
	}
	if err := store.CreateCart(ctx, seed.ShopID, weightedTokenHash[:]); err != nil {
		t.Fatalf("create MariaDB weight-value cart: %v", err)
	}
	if _, err := store.AddCartLine(ctx, seed.ShopID, weightedTokenHash[:], product.Variants[0].ID, 1); err != nil {
		t.Fatalf("add MariaDB weight-value cart line: %v", err)
	}
	quotes, err := store.ListCartShippingOptions(ctx, seed.ShopID, weightedTokenHash[:], "FR")
	if err != nil || len(quotes) != 1 || quotes[0].Amount != "4.5000" ||
		quotes[0].ServiceCode != "home" || quotes[0].RateVersion == "" ||
		quotes[0].WeightGrams == nil || *quotes[0].WeightGrams != 750 {
		t.Fatalf("MariaDB weight-value quote = %#v, error = %v", quotes, err)
	}
	weightedOrder, err := store.CheckoutCart(ctx, seed.ShopID, weightedTokenHash[:], CheckoutDetails{
		Email: weightedEmail, Name: "MariaDB Weight Value",
		ShippingCarrierCode: "colissimo", ShippingServiceCode: "home", ExpectedShippingTotal: quotes[0].Amount,
		ExpectedShippingQuoteID: quotes[0].QuoteID,
		ShippingAddress: ShippingAddress{
			Line1: "1 rue de test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout MariaDB weight-value cart: %v", err)
	}
	weightedOrderID = weightedOrder.ID
	if weightedOrder.ShippingRateMode != "weight_value" || weightedOrder.ShippingServiceCode != "home" ||
		weightedOrder.ShippingServiceName != "Domicile" || weightedOrder.ShippingRateVersion == "" || weightedOrder.ShippingWeightGrams == nil ||
		*weightedOrder.ShippingWeightGrams != 750 || len(weightedOrder.ShippingRateRule) == 0 {
		t.Fatalf("unexpected MariaDB weight-value order snapshot: %#v", weightedOrder)
	}
	previousRateVersion := weightedOrder.ShippingRateVersion
	updatedConfig, err := store.GetShippingConfig(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("read MariaDB shipping config for version update: %v", err)
	}
	updatedConfig.Rates[0].Rules[0].Amount = "4.60"
	if err := store.ReplaceShippingConfig(ctx, seed.ShopID, *updatedConfig); err != nil {
		t.Fatalf("save new MariaDB shipping rate version: %v", err)
	}
	var archivedVersionCount int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM shipping_rate_versions WHERE rate_version = $1`, previousRateVersion,
	).Scan(&archivedVersionCount); err != nil || archivedVersionCount != 1 {
		t.Fatalf("previous MariaDB rate version count = %d, error = %v; want 1", archivedVersionCount, err)
	}
	restoredVersion, err := store.RestoreShippingRateVersion(ctx, seed.ShopID, previousRateVersion)
	if err != nil || restoredVersion == previousRateVersion {
		t.Fatalf("restore MariaDB shipping rate version = %q, error = %v; want new version", restoredVersion, err)
	}
	restoredConfig, err := store.GetShippingConfig(ctx, seed.ShopID)
	if err != nil || len(restoredConfig.Rates) != 1 ||
		restoredConfig.Rates[0].RateVersion != restoredVersion ||
		restoredConfig.Rates[0].Rules[0].Amount != "4.5000" {
		t.Fatalf("restored MariaDB shipping configuration = %+v, error = %v", restoredConfig, err)
	}
	restoredOrder, err := store.GetAdminOrder(ctx, seed.ShopID, weightedOrderID)
	if err != nil || restoredOrder.ShippingRateVersion != previousRateVersion {
		t.Fatalf("MariaDB order snapshot after restore = %+v, error = %v; want version %s",
			restoredOrder, err, previousRateVersion)
	}

	brand, err := store.CreateBrand(ctx, seed.ShopID, BrandInput{Slug: "mariadb-brand-" + suffix, Name: "MariaDB Brand"})
	if err != nil {
		t.Fatalf("create MariaDB integration brand: %v", err)
	}
	personalizedBrandID = brand.ID
	supplier, err := store.CreateSupplier(ctx, seed.ShopID, SupplierInput{Code: "MDB-" + suffix, Name: "MariaDB Supplier", Active: true})
	if err != nil {
		t.Fatalf("create MariaDB integration supplier: %v", err)
	}
	personalizedSupplierID = supplier.ID
	personalizedProduct, err := store.CreateProduct(ctx, seed.ShopID, ProductInput{
		Slug: "mariadb-personalized-" + suffix, Status: "active",
		BrandID: brand.ID, SupplierIDs: []string{supplier.ID},
		PersonalizationOptions: []PersonalizationOption{{
			Code: "finish", Label: "Finish", Required: true,
			Choices: []PersonalizationChoice{{Value: "matte", Label: "Matte"}, {Value: "glossy", Label: "Glossy"}},
		}},
		Translations: []TranslationView{{Locale: "en-GB", Name: "MariaDB personalized product"}},
		Variants: []VariantInput{{
			SKU: "MDB-PERS-" + suffix, Options: map[string]any{}, WeightGrams: mariaDBInt64Pointer(500),
			Prices:    []PriceView{{Currency: "EUR", Amount: "15.0000"}},
			Inventory: InventoryView{Quantity: 5, Track: true},
			Suppliers: []VariantSupplierInput{{SupplierID: supplier.ID, SupplierSKU: "MDB-REF-" + suffix}},
		}},
	})
	if err != nil {
		t.Fatalf("create MariaDB personalized product: %v", err)
	}
	personalizedProductID = personalizedProduct.ID
	if personalizedProduct.Brand == nil || personalizedProduct.Brand.ID != brand.ID ||
		len(personalizedProduct.SupplierIDs) != 1 || len(personalizedProduct.Variants[0].Suppliers) != 1 {
		t.Fatalf("MariaDB catalog references = %+v", personalizedProduct)
	}
	if err := store.CreateCart(ctx, seed.ShopID, personalizedTokenHash[:]); err != nil {
		t.Fatalf("create MariaDB personalized cart: %v", err)
	}
	if _, err := store.AddCartLine(ctx, seed.ShopID, personalizedTokenHash[:], personalizedProduct.Variants[0].ID, 1); !errors.Is(err, ErrInvalidPersonalization) {
		t.Fatalf("MariaDB missing required selection error = %v, want %v", err, ErrInvalidPersonalization)
	}
	if _, err := store.AddCartLine(ctx, seed.ShopID, personalizedTokenHash[:], personalizedProduct.Variants[0].ID, 1, map[string]string{"finish": "matte"}); err != nil {
		t.Fatalf("add MariaDB personalized cart line: %v", err)
	}
	personalizedCart, err := store.GetCart(ctx, seed.ShopID, personalizedTokenHash[:])
	if err != nil || len(personalizedCart.Items) != 1 ||
		len(personalizedCart.Items[0].Personalizations) != 1 ||
		personalizedCart.Items[0].Personalizations[0].ChoiceLabel != "Matte" {
		t.Fatalf("MariaDB personalized cart = %+v, error = %v", personalizedCart, err)
	}
	personalizedQuotes, err := store.ListCartShippingOptions(ctx, seed.ShopID, personalizedTokenHash[:], "FR")
	if err != nil || len(personalizedQuotes) != 1 || personalizedQuotes[0].ServiceCode != "home" {
		t.Fatalf("MariaDB personalized shipping quote = %+v, error = %v", personalizedQuotes, err)
	}
	personalizedOrder, err := store.CheckoutCart(ctx, seed.ShopID, personalizedTokenHash[:], CheckoutDetails{
		Email: personalizedEmail, Name: "MariaDB Personalization",
		ShippingCarrierCode: "colissimo", ShippingServiceCode: "home",
		ExpectedShippingTotal:   personalizedQuotes[0].Amount,
		ExpectedShippingQuoteID: personalizedQuotes[0].QuoteID,
		ShippingAddress:         ShippingAddress{Line1: "1 rue de test", City: "Paris", PostalCode: "75001", Country: "FR"},
	})
	if err != nil || len(personalizedOrder.Items) != 1 ||
		len(personalizedOrder.Items[0].Personalizations) != 1 ||
		personalizedOrder.Items[0].Personalizations[0].ChoiceLabel != "Matte" ||
		len(personalizedOrder.Items[0].SupplierSnapshot) != 1 ||
		personalizedOrder.Items[0].SupplierSnapshot[0].Code != supplier.Code ||
		personalizedOrder.Items[0].SupplierSnapshot[0].SKU != "MDB-REF-"+suffix {
		t.Fatalf("MariaDB personalized order = %+v, error = %v", personalizedOrder, err)
	}
	personalizedOrderID = personalizedOrder.ID

	testSQLSupplierOrderSync(t, ctx, store)
}

func mariaDBInt64Pointer(value int64) *int64 { return &value }
