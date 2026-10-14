package sqlitestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"euromni/internal/seed"
	"euromni/internal/store"
)

func openSeededStore(t *testing.T, stock int) *Store {
	t.Helper()
	return openSeededStoreAt(t, filepath.Join(t.TempDir(), "sqlite", "shop.sqlite"), stock)
}

func openSeededStoreAt(t *testing.T, path string, stock int) *Store {
	t.Helper()
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open SQLite store: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close SQLite store: %v", err)
		}
	})

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO shops (id, default_locale, currency) VALUES (?, 'fr-FR', 'EUR')`, []any{seed.ShopID}},
		{`INSERT INTO categories (id, shop_id, slug) VALUES (?, ?, 'sacs')`, []any{seed.CategoryID, seed.ShopID}},
		{`INSERT INTO category_translations (category_id, locale, name, description) VALUES (?, 'fr-FR', 'Sacs', 'Sacs artisanaux')`, []any{seed.CategoryID}},
		{`INSERT INTO products (id, shop_id, slug, status) VALUES (?, ?, 'cabas-lin', 'active')`, []any{seed.ProductID, seed.ShopID}},
		{`INSERT INTO product_translations (product_id, locale, name, description) VALUES (?, 'fr-FR', 'Cabas en lin', 'Cabas quotidien')`, []any{seed.ProductID}},
		{`INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)`, []any{seed.ProductID, seed.CategoryID}},
		{`INSERT INTO variants (id, product_id, sku, options) VALUES (?, ?, 'CABAS-LIN-NAT', '{"color":"naturel"}')`, []any{seed.VariantID, seed.ProductID}},
		{`INSERT INTO prices (variant_id, currency, amount) VALUES (?, 'EUR', '89.0000')`, []any{seed.VariantID}},
		{`INSERT INTO inventory_items (variant_id, quantity, track) VALUES (?, ?, 1)`, []any{seed.VariantID, stock}},
	}
	for _, statement := range statements {
		if _, err := db.db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("seed SQLite store: %v", err)
		}
	}
	return db
}

func TestSQLiteStoreCatalogCartCheckoutFlow(t *testing.T) {
	ctx := context.Background()
	db := openSeededStore(t, 3)

	var (
		_ store.ProductReader  = db
		_ store.CategoryReader = db
		_ store.CartReader     = db
		_ store.CheckoutWriter = db
	)
	products, err := db.ListProducts(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("list SQLite products: %v", err)
	}
	if len(products) != 1 || products[0].Slug != "cabas-lin" || products[0].Variants[0].Prices[0].Amount != "89.0000" {
		t.Fatalf("products = %+v, want seeded cabas with exact decimal price", products)
	}
	categorized, err := db.ListProductsByCategory(ctx, seed.ShopID, seed.CategoryID)
	if err != nil || len(categorized) != 1 || categorized[0].ID != seed.ProductID {
		t.Fatalf("category products = %+v, error = %v", categorized, err)
	}
	category, err := db.GetCategory(ctx, seed.ShopID, seed.CategoryID)
	if err != nil || len(category.Translations) != 1 || category.Translations[0].Name != "Sacs" {
		t.Fatalf("category = %+v, error = %v", category, err)
	}

	tokenHash := sha256.Sum256([]byte("sqlite checkout cart"))
	if err := db.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create SQLite cart: %v", err)
	}
	cart, err := db.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 2)
	if err != nil {
		t.Fatalf("add SQLite cart line: %v", err)
	}
	if cart.Total.Amount != "178.0000" || len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Fatalf("cart = %+v, want two lines totalling EUR 178.0000", cart)
	}
	order, err := db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], store.CheckoutDetails{
		Email: " Buyer@Example.com ",
		Name:  "Client de test",
		ShippingAddress: store.ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("checkout SQLite cart: %v", err)
	}
	if order.Status != "pending" || order.Subtotal != "178.0000" || len(order.Items) != 1 || order.Items[0].Name != "Cabas en lin" {
		t.Fatalf("order = %+v, want pending snapshot order totalling EUR 178.0000", order)
	}
	var remainingStock int
	if err := db.db.QueryRowContext(ctx, `SELECT quantity FROM inventory_items WHERE variant_id = ?`, seed.VariantID).Scan(&remainingStock); err != nil {
		t.Fatalf("read stock after checkout: %v", err)
	}
	if remainingStock != 1 {
		t.Fatalf("remaining stock = %d, want 1", remainingStock)
	}
	emptyCart, err := db.GetCart(ctx, seed.ShopID, tokenHash[:])
	if err != nil || len(emptyCart.Items) != 0 || emptyCart.Total.Amount != "0.0000" {
		t.Fatalf("cart after checkout = %+v, error = %v; want empty cart", emptyCart, err)
	}
	if _, err := db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], store.CheckoutDetails{}); !errors.Is(err, store.ErrEmptyCart) {
		t.Fatalf("second checkout error = %v, want ErrEmptyCart", err)
	}
}

func TestSQLiteCheckoutSerializesStockConsumption(t *testing.T) {
	ctx := context.Background()
	db := openSeededStore(t, 1)
	tokenHashes := [][32]byte{
		sha256.Sum256([]byte("first sqlite cart")),
		sha256.Sum256([]byte("second sqlite cart")),
	}
	for _, tokenHash := range tokenHashes {
		if err := db.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
			t.Fatalf("create SQLite cart: %v", err)
		}
		if _, err := db.AddCartLine(ctx, seed.ShopID, tokenHash[:], seed.VariantID, 1); err != nil {
			t.Fatalf("add SQLite cart line: %v", err)
		}
	}

	results := make(chan error, len(tokenHashes))
	var wg sync.WaitGroup
	for _, tokenHash := range tokenHashes {
		wg.Add(1)
		go func(tokenHash [32]byte) {
			defer wg.Done()
			_, err := db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], store.CheckoutDetails{
				Email: "buyer@example.com",
				ShippingAddress: store.ShippingAddress{
					Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
				},
			})
			results <- err
		}(tokenHash)
	}
	wg.Wait()
	close(results)

	var succeeded, rejected int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, store.ErrOutOfStock):
			rejected++
		default:
			t.Fatalf("unexpected concurrent checkout error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent checkouts: succeeded=%d rejected=%d, want one of each", succeeded, rejected)
	}
	var remainingStock, orders int
	if err := db.db.QueryRowContext(ctx, `SELECT quantity FROM inventory_items WHERE variant_id = ?`, seed.VariantID).Scan(&remainingStock); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRowContext(ctx, `SELECT count(*) FROM orders`).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if remainingStock != 0 || orders != 1 {
		t.Fatalf("remaining stock=%d orders=%d, want 0 and 1", remainingStock, orders)
	}
}

func TestSQLiteCheckoutSerializesAcrossProcesses(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sqlite", "shop.sqlite")
	db := openSeededStoreAt(t, dbPath, 1)
	tokenHashes := [][32]byte{
		sha256.Sum256([]byte("first sqlite process cart")),
		sha256.Sum256([]byte("second sqlite process cart")),
	}
	for _, tokenHash := range tokenHashes {
		if err := db.CreateCart(context.Background(), seed.ShopID, tokenHash[:]); err != nil {
			t.Fatalf("create SQLite cart: %v", err)
		}
		if _, err := db.AddCartLine(context.Background(), seed.ShopID, tokenHash[:], seed.VariantID, 1); err != nil {
			t.Fatalf("add SQLite cart line: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close parent SQLite store: %v", err)
	}

	startFile := filepath.Join(t.TempDir(), "start")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find test executable: %v", err)
	}
	type childProcess struct {
		command *exec.Cmd
		output  *strings.Builder
	}
	children := make([]childProcess, 0, len(tokenHashes))
	for _, tokenHash := range tokenHashes {
		output := &strings.Builder{}
		command := exec.Command(executable, "-test.v", "-test.run=^TestSQLiteCheckoutProcessHelper$")
		command.Stdout = output
		command.Stderr = output
		command.Env = append(os.Environ(),
			"EUROMNI_SQLITESTORE_PROCESS_DB="+dbPath,
			"EUROMNI_SQLITESTORE_PROCESS_TOKEN="+fmt.Sprintf("%x", tokenHash),
			"EUROMNI_SQLITESTORE_PROCESS_START="+startFile,
		)
		if err := command.Start(); err != nil {
			t.Fatalf("start checkout process: %v", err)
		}
		children = append(children, childProcess{command: command, output: output})
	}
	if err := os.WriteFile(startFile, []byte("go"), 0o600); err != nil {
		t.Fatalf("release checkout processes: %v", err)
	}

	var succeeded, rejected int
	for _, child := range children {
		if err := child.command.Wait(); err != nil {
			t.Fatalf("checkout subprocess failed: %v\n%s", err, child.output.String())
		}
		switch {
		case strings.Contains(child.output.String(), "PROCESS_CHECKOUT_SUCCESS"):
			succeeded++
		case strings.Contains(child.output.String(), "PROCESS_CHECKOUT_OUT_OF_STOCK"):
			rejected++
		default:
			t.Fatalf("checkout subprocess returned no result:\n%s", child.output.String())
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent process checkouts: succeeded=%d rejected=%d, want one of each", succeeded, rejected)
	}

	db, err = Open(dbPath)
	if err != nil {
		t.Fatalf("reopen SQLite store: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close SQLite store: %v", err)
		}
	}()
	var remainingStock, orders int
	if err := db.db.QueryRow(`SELECT quantity FROM inventory_items WHERE variant_id = ?`, seed.VariantID).Scan(&remainingStock); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT count(*) FROM orders`).Scan(&orders); err != nil {
		t.Fatal(err)
	}
	if remainingStock != 0 || orders != 1 {
		t.Fatalf("remaining stock=%d orders=%d, want 0 and 1", remainingStock, orders)
	}
}

func TestSQLiteCheckoutProcessHelper(t *testing.T) {
	dbPath := os.Getenv("EUROMNI_SQLITESTORE_PROCESS_DB")
	if dbPath == "" {
		return
	}
	startFile := os.Getenv("EUROMNI_SQLITESTORE_PROCESS_START")
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err := os.Stat(startFile)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("wait for checkout release: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for checkout release")
		}
		time.Sleep(10 * time.Millisecond)
	}

	tokenHash, err := hex.DecodeString(os.Getenv("EUROMNI_SQLITESTORE_PROCESS_TOKEN"))
	if err != nil || len(tokenHash) != sha256.Size {
		t.Fatalf("decode cart token hash: invalid test input")
	}
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open SQLite store in subprocess: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close SQLite store in subprocess: %v", err)
		}
	}()
	_, err = db.CheckoutCart(context.Background(), seed.ShopID, tokenHash, store.CheckoutDetails{
		Email: "buyer@example.com",
		ShippingAddress: store.ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if errors.Is(err, store.ErrOutOfStock) {
		t.Log("PROCESS_CHECKOUT_OUT_OF_STOCK")
		return
	}
	if err != nil {
		t.Fatalf("checkout in subprocess: %v", err)
	}
	t.Log("PROCESS_CHECKOUT_SUCCESS")
}

func TestSQLiteStoreRejectsCrossShopAccess(t *testing.T) {
	ctx := context.Background()
	db := openSeededStore(t, 1)
	if _, err := db.GetProduct(ctx, "different-shop", seed.ProductID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-shop product error = %v, want ErrNotFound", err)
	}
	wrongShopToken := sha256.Sum256([]byte("wrong shop"))
	if err := db.CreateCart(ctx, "different-shop", wrongShopToken[:]); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-shop cart error = %v, want ErrNotFound", err)
	}
}
