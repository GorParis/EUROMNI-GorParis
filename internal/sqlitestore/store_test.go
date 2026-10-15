package sqlitestore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"euromni/internal/privatefs"
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

func TestSQLiteCatalogAdministration(t *testing.T) {
	ctx := context.Background()
	db := openSeededStore(t, 3)

	product, err := db.CreateProduct(ctx, seed.ShopID, store.ProductInput{
		Slug: "article-brouillon", Status: "draft",
		Translations: []store.TranslationView{{Locale: "fr-FR", Name: "Article brouillon", Description: "Texte"}},
		Media: []store.MediaInput{{
			URI: "/storefront/images/cabas-lin.svg", MimeType: "image/svg+xml", Alt: "Cabas local",
		}},
		Variants: []store.VariantInput{{
			SKU: "ARTICLE-BROUILLON", Options: map[string]any{"taille": "unique"},
			Prices:    []store.PriceView{{Currency: "EUR", Amount: "12.5000"}},
			Inventory: store.InventoryView{Quantity: 8, Track: true},
		}},
	})
	if err != nil {
		t.Fatalf("create SQLite product: %v", err)
	}
	if len(product.Variants) != 1 || product.Variants[0].Prices[0].Amount != "12.5000" || product.Variants[0].Inventory.Quantity != 8 {
		t.Fatalf("created SQLite variant, price, and inventory = %+v", product.Variants)
	}
	if len(product.Media) != 1 || product.Media[0].Position != 0 || product.Media[0].Alt != "Cabas local" {
		t.Fatalf("created SQLite media = %+v", product.Media)
	}
	mediaID := product.Media[0].ID
	products, err := db.ListAdminProducts(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("list admin SQLite products: %v", err)
	}
	if len(products) != 2 || products[0].ID != product.ID || products[0].Status != "draft" {
		t.Fatalf("admin products = %+v, want draft product included", products)
	}
	updated, err := db.UpdateProduct(ctx, seed.ShopID, product.ID, store.ProductInput{
		Slug: "article-brouillon", Status: "active",
		Translations: []store.TranslationView{{Locale: "fr-FR", Name: "Article publié", Description: "Mis à jour"}},
		Media: []store.MediaInput{
			{URI: "/storefront/images/cabas-lin.svg", MimeType: "image/svg+xml", Alt: "Cabas édité"},
			{URI: "https://cdn.example.test/detail.webp", MimeType: "image/webp", Alt: "Détail du cabas"},
		},
		Variants: []store.VariantInput{{
			SKU: "ARTICLE-BROUILLON", Options: map[string]any{"taille": "unique"},
			Prices:    []store.PriceView{{Currency: "EUR", Amount: "15.0000"}},
			Inventory: store.InventoryView{Quantity: 6, Track: true},
		}},
	})
	if err != nil {
		t.Fatalf("update SQLite product: %v", err)
	}
	publicProduct, err := db.GetProduct(ctx, seed.ShopID, product.ID)
	if err != nil || updated.Status != "active" || publicProduct.Translations[0].Name != "Article publié" ||
		len(publicProduct.Variants) != 1 || publicProduct.Variants[0].Prices[0].Amount != "15.0000" ||
		publicProduct.Variants[0].Inventory.Quantity != 6 || len(publicProduct.Media) != 2 ||
		publicProduct.Media[0].ID != mediaID || publicProduct.Media[0].Position != 0 ||
		publicProduct.Media[0].Alt != "Cabas édité" || publicProduct.Media[1].Position != 1 {
		t.Fatalf("updated/public product = %+v/%+v, error = %v", updated, publicProduct, err)
	}

	category, err := db.CreateCategory(ctx, seed.ShopID, store.CategoryInput{
		Slug:         "accessoires",
		Translations: []store.CategoryTranslationView{{Locale: "fr-FR", Name: "Accessoires", Description: "Objets"}},
	})
	if err != nil {
		t.Fatalf("create SQLite category: %v", err)
	}
	if _, err := db.CreateCategory(ctx, seed.ShopID, store.CategoryInput{Slug: "accessoires"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate SQLite category error = %v, want %v", err, store.ErrConflict)
	}
	updatedCategory, err := db.UpdateCategory(ctx, seed.ShopID, category.ID, store.CategoryInput{
		Slug:         "accessoires-mode",
		Translations: []store.CategoryTranslationView{{Locale: "fr-FR", Name: "Accessoires mode"}},
	})
	if err != nil || updatedCategory.Slug != "accessoires-mode" {
		t.Fatalf("updated SQLite category = %+v, error = %v", updatedCategory, err)
	}
	updated, err = db.UpdateProduct(ctx, seed.ShopID, product.ID, store.ProductInput{
		Slug: "article-brouillon", Status: "active",
		Translations: []store.TranslationView{{Locale: "fr-FR", Name: "Article publié"}},
		CategoryIDs:  []string{category.ID},
		Media: []store.MediaInput{
			{URI: "/storefront/images/cabas-lin.svg", MimeType: "image/svg+xml", Alt: "Cabas édité"},
		},
		Variants: []store.VariantInput{{
			SKU: "ARTICLE-BROUILLON", Options: map[string]any{"taille": "unique"},
			Prices:    []store.PriceView{{Currency: "EUR", Amount: "15.0000"}},
			Inventory: store.InventoryView{Quantity: 6, Track: true},
		}},
	})
	if err != nil || len(updated.CategoryIDs) != 1 || updated.CategoryIDs[0] != category.ID {
		t.Fatalf("SQLite category association = %+v, error = %v", updated, err)
	}
	cartToken := sha256.Sum256([]byte("sqlite admin variant reference cart"))
	if err := db.CreateCart(ctx, seed.ShopID, cartToken[:]); err != nil {
		t.Fatalf("create SQLite variant reference cart: %v", err)
	}
	defer func() {
		_, _ = db.db.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = ?", cartToken[:])
	}()
	if _, err := db.AddCartLine(ctx, seed.ShopID, cartToken[:], updated.Variants[0].ID, 1); err != nil {
		t.Fatalf("add SQLite admin variant to cart: %v", err)
	}
	if _, err := db.UpdateProduct(ctx, seed.ShopID, product.ID, store.ProductInput{
		Slug: "article-brouillon", Status: "active",
		Translations: []store.TranslationView{{Locale: "fr-FR", Name: "Article publié"}},
	}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("removing cart-referenced SQLite variant error = %v, want %v", err, store.ErrConflict)
	}
	if err := db.DeleteCategory(ctx, seed.ShopID, category.ID); err != nil {
		t.Fatalf("delete SQLite category: %v", err)
	}
	if _, err := db.GetCategory(ctx, seed.ShopID, category.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted SQLite category error = %v, want %v", err, store.ErrNotFound)
	}
	if err := db.ArchiveProduct(ctx, seed.ShopID, product.ID); err != nil {
		t.Fatalf("archive SQLite product: %v", err)
	}
	if _, err := db.GetProduct(ctx, seed.ShopID, product.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("archived SQLite product error = %v, want %v", err, store.ErrNotFound)
	}
}

func TestSQLiteMigrationsRecordAndUpgradeExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sqlite", "shop.sqlite")
	db := openSeededStoreAt(t, path, 2)
	var version int
	if err := db.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read SQLite migration version: %v", err)
	}
	if version != 1 {
		t.Fatalf("migration version = %d, want 1", version)
	}
	if _, err := db.db.Exec(`DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("remove migration ledger to simulate prototype database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close SQLite store: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade existing SQLite database: %v", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("close upgraded SQLite store: %v", err)
		}
	}()
	if err := reopened.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read upgraded SQLite migration version: %v", err)
	}
	if version != 1 {
		t.Fatalf("upgraded migration version = %d, want 1", version)
	}
	var shopCount int
	if err := reopened.db.QueryRow(`SELECT count(*) FROM shops WHERE id = ?`, seed.ShopID).Scan(&shopCount); err != nil {
		t.Fatalf("read existing shop after migration: %v", err)
	}
	if shopCount != 1 {
		t.Fatalf("existing shop count after migration = %d, want 1", shopCount)
	}
}

func TestSQLiteRejectsDatabaseFromNewerSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sqlite", "shop.sqlite")
	db := openSeededStoreAt(t, path, 1)
	if _, err := db.db.Exec(`INSERT INTO schema_migrations (version, applied_at) VALUES (99, ?)`, time.Now().Unix()); err != nil {
		t.Fatalf("set newer SQLite schema version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close SQLite store: %v", err)
	}
	if _, err := Open(path); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("opening newer SQLite database error = %v, want clear version error", err)
	}
}

func TestSQLiteConcurrentInitializationAppliesMigrationsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sqlite", "shop.sqlite")
	type openResult struct {
		store *Store
		err   error
	}
	results := make(chan openResult, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			db, err := Open(path)
			results <- openResult{store: db, err: err}
		}()
	}
	close(start)
	var stores []*Store
	var openErr error
	for range 2 {
		result := <-results
		if result.err != nil {
			openErr = errors.Join(openErr, result.err)
			continue
		}
		stores = append(stores, result.store)
	}
	for _, db := range stores {
		if err := db.Close(); err != nil {
			t.Errorf("close concurrently initialized SQLite store: %v", err)
		}
	}
	if openErr != nil {
		t.Fatalf("concurrent SQLite initialization: %v", openErr)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("reopen concurrently initialized SQLite store: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close SQLite store: %v", err)
		}
	}()
	var migrationCount int
	if err := db.db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count SQLite migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("recorded migrations = %d, want 1", migrationCount)
	}
}

func TestSQLiteBackupIncludesWALDataAndDoesNotOverwrite(t *testing.T) {
	db := openSeededStore(t, 1)
	tokenHash := sha256.Sum256([]byte("SQLite backup cart"))
	if err := db.CreateCart(context.Background(), seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart before backup: %v", err)
	}
	if _, err := db.AddCartLine(context.Background(), seed.ShopID, tokenHash[:], seed.VariantID, 1); err != nil {
		t.Fatalf("add cart line before backup: %v", err)
	}
	walInfo, err := os.Stat(db.path + "-wal")
	if err != nil {
		t.Fatalf("inspect SQLite WAL before backup: %v", err)
	}
	if walInfo.Size() == 0 {
		t.Fatal("SQLite WAL is empty before backup")
	}

	backupPath := filepath.Join(t.TempDir(), "backup", "snapshot.sqlite")
	if err := db.Backup(context.Background(), db.path); err == nil {
		t.Fatal("backup accepted the source database path")
	}
	if err := db.Backup(context.Background(), backupPath); err != nil {
		t.Fatalf("create SQLite backup: %v", err)
	}
	restorePath := filepath.Join(t.TempDir(), "restored", "shop.sqlite")
	if err := privatefs.EnsureDirectory(filepath.Dir(restorePath)); err != nil {
		t.Fatalf("create SQLite restore directory: %v", err)
	}
	backupBytes, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read SQLite backup for restore: %v", err)
	}
	if err := os.WriteFile(restorePath, backupBytes, 0o600); err != nil {
		t.Fatalf("restore SQLite backup: %v", err)
	}
	restored, err := Open(restorePath)
	if err != nil {
		t.Fatalf("open restored SQLite database: %v", err)
	}
	var cartCount, lineCount int
	if err := restored.db.QueryRow(`SELECT count(*) FROM carts`).Scan(&cartCount); err != nil {
		t.Fatalf("read restored carts: %v", err)
	}
	if err := restored.db.QueryRow(`SELECT count(*) FROM cart_lines`).Scan(&lineCount); err != nil {
		t.Fatalf("read restored cart lines: %v", err)
	}
	if cartCount != 1 || lineCount != 1 {
		t.Fatalf("restored database has carts=%d lines=%d, want 1 and 1", cartCount, lineCount)
	}
	backupInfo, err := os.Stat(restorePath)
	if err != nil {
		t.Fatalf("inspect restored SQLite database: %v", err)
	}
	if runtime.GOOS != "windows" && backupInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("restored SQLite database permissions = %o, want private permissions", backupInfo.Mode().Perm())
	}
	if err := restored.Close(); err != nil {
		t.Fatalf("close restored SQLite database: %v", err)
	}
	temporaryBackups, err := filepath.Glob(filepath.Join(filepath.Dir(backupPath), ".euromni-backup-*.sqlite"))
	if err != nil {
		t.Fatalf("find temporary SQLite backups: %v", err)
	}
	if len(temporaryBackups) != 0 {
		t.Fatalf("temporary SQLite backups remain: %v", temporaryBackups)
	}

	if err := os.WriteFile(backupPath, []byte("keep existing file"), 0o600); err != nil {
		t.Fatalf("replace backup with sentinel for overwrite test: %v", err)
	}
	if err := db.Backup(context.Background(), backupPath); err == nil {
		t.Fatal("backup overwrote an existing file")
	}
	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read sentinel backup destination: %v", err)
	}
	if string(content) != "keep existing file" {
		t.Fatalf("existing destination contents changed: %q", content)
	}
}

func TestSQLiteCancelledCheckoutRollsBack(t *testing.T) {
	db := openSeededStore(t, 1)
	tokenHash := sha256.Sum256([]byte("cancelled SQLite checkout"))
	if err := db.CreateCart(context.Background(), seed.ShopID, tokenHash[:]); err != nil {
		t.Fatalf("create cart before cancelled checkout: %v", err)
	}
	if _, err := db.AddCartLine(context.Background(), seed.ShopID, tokenHash[:], seed.VariantID, 1); err != nil {
		t.Fatalf("add cart line before cancelled checkout: %v", err)
	}

	lockerDB, err := sql.Open("sqlite", db.path+"?_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		t.Fatalf("open SQLite lock connection: %v", err)
	}
	lockerDB.SetMaxOpenConns(1)
	locker, err := lockerDB.Conn(context.Background())
	if err != nil {
		_ = lockerDB.Close()
		t.Fatalf("get SQLite lock connection: %v", err)
	}
	if _, err := locker.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
		_ = locker.Close()
		_ = lockerDB.Close()
		t.Fatalf("hold SQLite write lock: %v", err)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			if _, err := locker.ExecContext(context.Background(), `ROLLBACK`); err != nil {
				t.Errorf("release SQLite write lock: %v", err)
			}
		}
		if err := locker.Close(); err != nil {
			t.Errorf("close SQLite lock connection: %v", err)
		}
		if err := lockerDB.Close(); err != nil {
			t.Errorf("close SQLite lock database: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	_, err = db.CheckoutCart(ctx, seed.ShopID, tokenHash[:], store.CheckoutDetails{
		Email: "buyer@example.com",
		ShippingAddress: store.ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled checkout error = %v, want context deadline exceeded", err)
	}
	if _, err := locker.ExecContext(context.Background(), `ROLLBACK`); err != nil {
		t.Fatalf("release SQLite write lock after cancelled checkout: %v", err)
	}
	lockHeld = false

	var remainingStock, orderCount, cartLineCount int
	if err := db.db.QueryRow(`SELECT quantity FROM inventory_items WHERE variant_id = ?`, seed.VariantID).Scan(&remainingStock); err != nil {
		t.Fatalf("read stock after cancelled checkout: %v", err)
	}
	if err := db.db.QueryRow(`SELECT count(*) FROM orders`).Scan(&orderCount); err != nil {
		t.Fatalf("count orders after cancelled checkout: %v", err)
	}
	if err := db.db.QueryRow(`SELECT count(*) FROM cart_lines WHERE cart_id = (SELECT id FROM carts WHERE access_token_hash = ?)`, tokenHash[:]).Scan(&cartLineCount); err != nil {
		t.Fatalf("count cart lines after cancelled checkout: %v", err)
	}
	if remainingStock != 1 || orderCount != 0 || cartLineCount != 1 {
		t.Fatalf("after cancelled checkout stock=%d orders=%d cart_lines=%d, want 1, 0, 1", remainingStock, orderCount, cartLineCount)
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
