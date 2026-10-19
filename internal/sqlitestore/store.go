package sqlitestore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"euromni/internal/privatefs"
	"euromni/internal/store"
	"modernc.org/sqlite"
)

//go:embed migrations/*.sql
var schema embed.FS

const maxCartLineQuantity = 99
const sqliteBusyTimeoutMilliseconds = 25
const sqliteWriteLockWaitTimeout = 5 * time.Second

type Store struct {
	db   *sql.DB
	path string
	mu   sync.Mutex
}

var (
	_ store.ProductReader         = (*Store)(nil)
	_ store.ProductCategoryReader = (*Store)(nil)
	_ store.CategoryReader        = (*Store)(nil)
	_ store.CartReader            = (*Store)(nil)
	_ store.CheckoutWriter        = (*Store)(nil)
)

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("SQLite database path is required")
	}
	if path != ":memory:" {
		absolutePath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve SQLite database path: %w", err)
		}
		if err := privatefs.EnsureDirectory(filepath.Dir(absolutePath)); err != nil {
			return nil, fmt.Errorf("secure SQLite database directory: %w", err)
		}
		path = absolutePath
	}
	databasePath := path

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(%d)&_pragma=foreign_keys(ON)&_txlock=immediate", path, sqliteBusyTimeoutMilliseconds)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx := context.Background()
	if err := retrySQLiteBusy(ctx, func() error { return db.PingContext(ctx) }); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to SQLite database: %w", err)
	}
	if path != ":memory:" {
		if err := retrySQLiteBusy(ctx, func() error {
			var journalMode string
			if err := db.QueryRowContext(ctx, `PRAGMA journal_mode=WAL`).Scan(&journalMode); err != nil {
				return err
			}
			if !strings.EqualFold(journalMode, "wal") {
				return fmt.Errorf("SQLite journal mode is %q, want WAL", journalMode)
			}
			return nil
		}); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure SQLite WAL journal: %w", err)
		}
	}
	s := &Store{db: db, path: databasePath}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize SQLite schema: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(schema, "migrations")
	if err != nil {
		return err
	}
	type migration struct {
		version int
		name    string
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		versionText := strings.SplitN(entry.Name(), "_", 2)[0]
		version, err := strconv.Atoi(versionText)
		if err != nil || version < 1 {
			return fmt.Errorf("invalid SQLite migration filename %q", entry.Name())
		}
		migrations = append(migrations, migration{version: version, name: entry.Name()})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for i := 1; i < len(migrations); i++ {
		if migrations[i].version == migrations[i-1].version {
			return fmt.Errorf("duplicate SQLite migration version %d", migrations[i].version)
		}
	}

	ledgerTx, err := s.beginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin SQLite migration ledger setup: %w", err)
	}
	if _, err := ledgerTx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		_ = ledgerTx.Rollback()
		return fmt.Errorf("create SQLite migration ledger: %w", err)
	}
	if err := ledgerTx.Commit(); err != nil {
		return fmt.Errorf("commit SQLite migration ledger setup: %w", err)
	}
	var latestVersion int
	if len(migrations) > 0 {
		latestVersion = migrations[len(migrations)-1].version
	}
	var currentVersion int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&currentVersion); err != nil {
		return fmt.Errorf("read SQLite migration version: %w", err)
	}
	if currentVersion > latestVersion {
		return fmt.Errorf("SQLite database schema version %d is newer than supported version %d", currentVersion, latestVersion)
	}
	for _, migration := range migrations {
		statements, err := schema.ReadFile(filepath.ToSlash(filepath.Join("migrations", migration.name)))
		if err != nil {
			return fmt.Errorf("read SQLite migration %q: %w", migration.name, err)
		}
		tx, err := s.beginTx(ctx)
		if err != nil {
			return fmt.Errorf("begin SQLite migration %q: %w", migration.name, err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&currentVersion); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("read SQLite migration version before %q: %w", migration.name, err)
		}
		if currentVersion > latestVersion {
			_ = tx.Rollback()
			return fmt.Errorf("SQLite database schema version %d is newer than supported version %d", currentVersion, latestVersion)
		}
		if migration.version <= currentVersion {
			if err := tx.Rollback(); err != nil {
				return fmt.Errorf("finish SQLite migration check %q: %w", migration.name, err)
			}
			continue
		}
		if migration.version != currentVersion+1 {
			_ = tx.Rollback()
			return fmt.Errorf("SQLite migration sequence missing version %d", currentVersion+1)
		}
		if _, err := tx.ExecContext(ctx, string(statements)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply SQLite migration %q: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, migration.version, time.Now().Unix()); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record SQLite migration %q: %w", migration.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit SQLite migration %q: %w", migration.name, err)
		}
	}
	return nil
}

func (s *Store) beginTx(ctx context.Context) (*sql.Tx, error) {
	var tx *sql.Tx
	err := retrySQLiteBusy(ctx, func() error {
		var err error
		tx, err = s.db.BeginTx(ctx, nil)
		return err
	})
	return tx, err
}

func retrySQLiteBusy(ctx context.Context, operation func() error) error {
	deadline := time.Now().Add(sqliteWriteLockWaitTimeout)
	for {
		err := operation()
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		var sqliteErr *sqlite.Error
		if !errors.As(err, &sqliteErr) || sqliteErr.Code()&0xff != 5 {
			return err
		}
		wait := time.Until(deadline)
		if wait <= 0 {
			return err
		}
		if wait > sqliteBusyTimeoutMilliseconds*time.Millisecond {
			wait = sqliteBusyTimeoutMilliseconds * time.Millisecond
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Backup writes a consistent SQLite snapshot to a new file without overwriting existing data.
func (s *Store) Backup(ctx context.Context, destination string) error {
	if s == nil || s.db == nil {
		return errors.New("SQLite store is not initialized")
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("SQLite backup destination is required")
	}
	if ctx == nil {
		return errors.New("SQLite backup context is required")
	}
	absoluteDestination, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("resolve SQLite backup destination: %w", err)
	}
	absoluteDestination = filepath.Clean(absoluteDestination)
	if s.path != ":memory:" && samePath(absoluteDestination, s.path) {
		return errors.New("SQLite backup destination must differ from the database path")
	}
	if err := privatefs.EnsureDirectory(filepath.Dir(absoluteDestination)); err != nil {
		return fmt.Errorf("secure SQLite backup directory: %w", err)
	}
	if _, err := os.Lstat(absoluteDestination); err == nil {
		return fmt.Errorf("SQLite backup destination already exists: %s", absoluteDestination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect SQLite backup destination: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tempFile, err := os.CreateTemp(filepath.Dir(absoluteDestination), ".euromni-backup-*.sqlite")
	if err != nil {
		return fmt.Errorf("reserve temporary SQLite backup path: %w", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temporary SQLite backup file: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("prepare temporary SQLite backup path: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, tempPath); err != nil {
		if cleanupErr := os.Remove(tempPath); cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
			return errors.Join(fmt.Errorf("create SQLite backup: %w", err), fmt.Errorf("remove incomplete SQLite backup: %w", cleanupErr))
		}
		return fmt.Errorf("create SQLite backup: %w", err)
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		if cleanupErr := os.Remove(tempPath); cleanupErr != nil {
			return errors.Join(fmt.Errorf("restrict SQLite backup permissions: %w", err), fmt.Errorf("remove SQLite backup: %w", cleanupErr))
		}
		return fmt.Errorf("restrict SQLite backup permissions: %w", err)
	}
	if err := os.Link(tempPath, absoluteDestination); err != nil {
		if cleanupErr := os.Remove(tempPath); cleanupErr != nil {
			return errors.Join(fmt.Errorf("publish SQLite backup: %w", err), fmt.Errorf("remove temporary SQLite backup: %w", cleanupErr))
		}
		return fmt.Errorf("publish SQLite backup: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("SQLite backup created at %s but temporary backup cleanup failed: %w", absoluteDestination, err)
	}
	return nil
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if filepath.VolumeName(a) != "" || filepath.VolumeName(b) != "" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func (s *Store) ListProducts(ctx context.Context, shopID string) ([]store.ProductView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM products
		WHERE shop_id = ? AND status = 'active'
		ORDER BY slug, id
	`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	products := make([]store.ProductView, 0, len(ids))
	for _, id := range ids {
		product, err := s.GetProduct(ctx, shopID, id)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
	}
	return products, nil
}

func (s *Store) ListProductsByCategory(ctx context.Context, shopID, categoryID string) ([]store.ProductView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id
		FROM products p
		JOIN product_categories pc ON pc.product_id = p.id
		JOIN categories c ON c.id = pc.category_id AND c.shop_id = p.shop_id
		WHERE p.shop_id = ? AND p.status = 'active' AND c.id = ?
		ORDER BY p.slug, p.id
	`, shopID, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	products := make([]store.ProductView, 0, len(ids))
	for _, id := range ids {
		product, err := s.GetProduct(ctx, shopID, id)
		if err != nil {
			return nil, err
		}
		products = append(products, *product)
	}
	return products, nil
}

func (s *Store) GetProduct(ctx context.Context, shopID, productID string) (*store.ProductView, error) {
	return s.getProduct(ctx, shopID, productID, true)
}

func (s *Store) getProduct(ctx context.Context, shopID, productID string, activeOnly bool) (*store.ProductView, error) {
	var product store.ProductView
	var brandID sql.NullString
	var personalizationOptions []byte
	query := `
		SELECT id, slug, status, brand_id, personalization_options FROM products
		WHERE shop_id = ? AND id = ?
	`
	if activeOnly {
		query += ` AND status = 'active'`
	}
	if err := s.db.QueryRowContext(ctx, query, shopID, productID).Scan(
		&product.ID, &product.Slug, &product.Status, &brandID, &personalizationOptions,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	product.PersonalizationOptions = make([]store.PersonalizationOption, 0)
	if err := json.Unmarshal(personalizationOptions, &product.PersonalizationOptions); err != nil {
		return nil, fmt.Errorf("decode personalization options: %w", err)
	}
	if product.PersonalizationOptions == nil {
		product.PersonalizationOptions = []store.PersonalizationOption{}
	}
	if brandID.Valid {
		var brand store.BrandView
		if err := s.db.QueryRowContext(ctx, `
			SELECT id, slug, name FROM brands WHERE shop_id = ? AND id = ?
		`, shopID, brandID.String).Scan(&brand.ID, &brand.Slug, &brand.Name); err != nil {
			return nil, err
		}
		product.Brand = &brand
	}
	product.SupplierIDs = make([]string, 0)
	supplierRows, err := s.db.QueryContext(ctx, `
		SELECT product_supplier.supplier_id
		FROM product_suppliers product_supplier
		JOIN suppliers supplier
		  ON supplier.id = product_supplier.supplier_id AND supplier.shop_id = ?
		WHERE product_supplier.product_id = ?
		ORDER BY product_supplier.supplier_id
	`, shopID, productID)
	if err != nil {
		return nil, err
	}
	for supplierRows.Next() {
		var supplierID string
		if err := supplierRows.Scan(&supplierID); err != nil {
			_ = supplierRows.Close()
			return nil, err
		}
		product.SupplierIDs = append(product.SupplierIDs, supplierID)
	}
	if err := supplierRows.Err(); err != nil {
		_ = supplierRows.Close()
		return nil, err
	}
	if err := supplierRows.Close(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT locale, name, description
		FROM product_translations WHERE product_id = ? ORDER BY locale
	`, productID)
	if err != nil {
		return nil, err
	}
	product.Translations = make([]store.TranslationView, 0)
	for rows.Next() {
		var translation store.TranslationView
		if err := rows.Scan(&translation.Locale, &translation.Name, &translation.Description); err != nil {
			_ = rows.Close()
			return nil, err
		}
		product.Translations = append(product.Translations, translation)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT category_id
		FROM product_categories
		WHERE product_id = ?
		ORDER BY category_id
	`, productID)
	if err != nil {
		return nil, err
	}
	product.CategoryIDs = make([]string, 0)
	for rows.Next() {
		var categoryID string
		if err := rows.Scan(&categoryID); err != nil {
			_ = rows.Close()
			return nil, err
		}
		product.CategoryIDs = append(product.CategoryIDs, categoryID)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT m.id, pm.position, m.uri, m.mime_type, m.alt
		FROM product_media pm JOIN media m ON m.id = pm.media_id
		WHERE pm.product_id = ? AND m.shop_id = ?
		ORDER BY pm.position, m.id
	`, productID, shopID)
	if err != nil {
		return nil, err
	}
	product.Media = make([]store.MediaView, 0)
	for rows.Next() {
		var media store.MediaView
		if err := rows.Scan(&media.ID, &media.Position, &media.URI, &media.MimeType, &media.Alt); err != nil {
			_ = rows.Close()
			return nil, err
		}
		product.Media = append(product.Media, media)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT v.id, v.sku, v.options, COALESCE(i.quantity, 0), COALESCE(i.track, 1)
		FROM variants v LEFT JOIN inventory_items i ON i.variant_id = v.id
		WHERE v.product_id = ? ORDER BY v.sku, v.id
	`, productID)
	if err != nil {
		return nil, err
	}
	product.Variants = make([]store.VariantView, 0)
	for rows.Next() {
		var variant store.VariantView
		var options []byte
		if err := rows.Scan(&variant.ID, &variant.SKU, &options, &variant.Inventory.Quantity, &variant.Inventory.Track); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(options, &variant.Options); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("decode variant options: %w", err)
		}
		if variant.Options == nil {
			variant.Options = make(map[string]any)
		}
		product.Variants = append(product.Variants, variant)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range product.Variants {
		prices, err := s.pricesFor(ctx, product.Variants[i].ID)
		if err != nil {
			return nil, err
		}
		product.Variants[i].Prices = prices
		product.Variants[i].Suppliers, err = s.variantSuppliersFor(ctx, shopID, product.Variants[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return &product, nil
}

func (s *Store) variantSuppliersFor(ctx context.Context, shopID, variantID string) ([]store.VariantSupplierView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT supplier.id, supplier.name, variant_supplier.supplier_sku
		FROM variant_suppliers variant_supplier
		JOIN suppliers supplier ON supplier.id = variant_supplier.supplier_id
		JOIN variants variant ON variant.id = variant_supplier.variant_id
		JOIN products product ON product.id = variant.product_id
		WHERE variant_supplier.variant_id = ? AND product.shop_id = ? AND supplier.shop_id = ?
		ORDER BY supplier.code, supplier.id
	`, variantID, shopID, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	suppliers := make([]store.VariantSupplierView, 0)
	for rows.Next() {
		var supplier store.VariantSupplierView
		if err := rows.Scan(&supplier.SupplierID, &supplier.SupplierName, &supplier.SupplierSKU); err != nil {
			return nil, err
		}
		suppliers = append(suppliers, supplier)
	}
	return suppliers, rows.Err()
}

func (s *Store) pricesFor(ctx context.Context, variantID string) ([]store.PriceView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT currency, amount, compare_at FROM prices
		WHERE variant_id = ? ORDER BY currency
	`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prices := make([]store.PriceView, 0)
	for rows.Next() {
		var price store.PriceView
		var compareAt sql.NullString
		if err := rows.Scan(&price.Currency, &price.Amount, &compareAt); err != nil {
			return nil, err
		}
		if compareAt.Valid {
			price.CompareAt = &compareAt.String
		}
		prices = append(prices, price)
	}
	return prices, rows.Err()
}

func (s *Store) ListCategories(ctx context.Context, shopID string) ([]store.CategoryView, error) {
	return s.readCategories(ctx, shopID, "")
}

func (s *Store) GetCategory(ctx context.Context, shopID, categoryID string) (*store.CategoryView, error) {
	categories, err := s.readCategories(ctx, shopID, categoryID)
	if err != nil {
		return nil, err
	}
	if len(categories) == 0 {
		return nil, store.ErrNotFound
	}
	return &categories[0], nil
}

func (s *Store) readCategories(ctx context.Context, shopID, categoryID string) ([]store.CategoryView, error) {
	query := `
		SELECT c.id, c.slug, ct.locale, ct.name, ct.description
		FROM categories c LEFT JOIN category_translations ct ON ct.category_id = c.id
		WHERE c.shop_id = ?
	`
	args := []any{shopID}
	if categoryID != "" {
		query += ` AND c.id = ?`
		args = append(args, categoryID)
	}
	query += ` ORDER BY c.position, c.id, ct.locale`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	categories := make([]store.CategoryView, 0)
	indexes := make(map[string]int)
	for rows.Next() {
		var id, slug string
		var locale, name, description sql.NullString
		if err := rows.Scan(&id, &slug, &locale, &name, &description); err != nil {
			return nil, err
		}
		index, exists := indexes[id]
		if !exists {
			index = len(categories)
			indexes[id] = index
			categories = append(categories, store.CategoryView{
				ID: id, Slug: slug, Translations: make([]store.CategoryTranslationView, 0),
			})
		}
		if locale.Valid {
			categories[index].Translations = append(categories[index].Translations, store.CategoryTranslationView{
				Locale: locale.String, Name: name.String, Description: description.String,
			})
		}
	}
	return categories, rows.Err()
}

func (s *Store) CreateCart(ctx context.Context, shopID string, tokenHash []byte) error {
	if len(tokenHash) != 32 {
		return store.ErrInvalidCartToken
	}
	id, err := newUUID()
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO carts (id, shop_id, currency, access_token_hash, access_token_expires_at)
		SELECT ?, id, currency, ?, ? FROM shops WHERE id = ?
	`, id, tokenHash, time.Now().Add(30*24*time.Hour).Unix(), shopID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) GetCart(ctx context.Context, shopID string, tokenHash []byte) (*store.CartView, error) {
	if len(tokenHash) != 32 {
		return nil, store.ErrInvalidCartToken
	}
	var cartID, currency string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, currency FROM carts
		WHERE shop_id = ? AND access_token_hash = ? AND access_token_expires_at > ?
	`, shopID, tokenHash, time.Now().Unix()).Scan(&cartID, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrInvalidCartToken
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.id, v.sku, cl.quantity, p.amount, cl.personalizations
		FROM cart_lines cl
		JOIN variants v ON v.id = cl.variant_id
		JOIN products pdt ON pdt.id = v.product_id
		JOIN prices p ON p.variant_id = v.id AND p.currency = ?
		WHERE cl.cart_id = ? AND pdt.shop_id = ? AND pdt.status = 'active'
		ORDER BY v.sku, v.id
	`, currency, cartID, shopID)
	if err != nil {
		return nil, err
	}
	items := make([]store.CartLineView, 0)
	total := new(big.Rat)
	for rows.Next() {
		var item store.CartLineView
		var personalizations []byte
		if err := rows.Scan(&item.VariantID, &item.SKU, &item.Quantity, &item.UnitAmount, &personalizations); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if err := json.Unmarshal(personalizations, &item.Personalizations); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if item.Personalizations == nil {
			item.Personalizations = []store.PersonalizationSelection{}
		}
		price, ok := new(big.Rat).SetString(item.UnitAmount)
		if !ok {
			_ = rows.Close()
			return nil, errors.New("invalid product price")
		}
		total.Add(total, new(big.Rat).Mul(price, big.NewRat(int64(item.Quantity), 1)))
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return &store.CartView{
		Items: items,
		Total: store.CartTotalView{Amount: total.FloatString(4), Currency: currency},
	}, nil
}

func (s *Store) AddCartLine(ctx context.Context, shopID string, tokenHash []byte, variantID string, quantity int, submittedSelections ...map[string]string) (*store.CartView, error) {
	if len(tokenHash) != 32 {
		return nil, store.ErrInvalidCartToken
	}
	if quantity < 1 || quantity > maxCartLineQuantity {
		return nil, store.ErrInvalidCartQuantity
	}
	submitted := map[string]string{}
	if len(submittedSelections) > 0 && submittedSelections[0] != nil {
		submitted = submittedSelections[0]
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.beginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var cartID, currency string
	err = tx.QueryRowContext(ctx, `
		SELECT id, currency FROM carts
		WHERE shop_id = ? AND access_token_hash = ? AND access_token_expires_at > ?
	`, shopID, tokenHash, time.Now().Unix()).Scan(&cartID, &currency)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrInvalidCartToken
	}
	if err != nil {
		return nil, err
	}
	var inventory, tracked int
	var optionJSON []byte
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(i.quantity, 0), COALESCE(i.track, 1), p.personalization_options
		FROM variants v
		JOIN products p ON p.id = v.product_id
		JOIN prices pr ON pr.variant_id = v.id AND pr.currency = ?
		LEFT JOIN inventory_items i ON i.variant_id = v.id
		WHERE v.id = ? AND p.shop_id = ? AND p.status = 'active'
	`, currency, variantID, shopID).Scan(&inventory, &tracked, &optionJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var options []store.PersonalizationOption
	if err := json.Unmarshal(optionJSON, &options); err != nil {
		return nil, err
	}
	_, snapshot, personalizationHash, err := store.ValidatePersonalizationSelections(options, submitted)
	if err != nil {
		return nil, err
	}
	var current int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity), 0) FROM cart_lines WHERE cart_id = ? AND variant_id = ?
	`, cartID, variantID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if current+quantity > maxCartLineQuantity {
		return nil, store.ErrInvalidCartQuantity
	}
	if tracked != 0 && current+quantity > inventory {
		return nil, store.ErrOutOfStock
	}
	var currentLine int
	err = tx.QueryRowContext(ctx, `
		SELECT quantity FROM cart_lines
		WHERE cart_id = ? AND variant_id = ? AND personalization_hash = ?
	`, cartID, variantID, personalizationHash).Scan(&currentLine)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if currentLine+quantity > maxCartLineQuantity {
		return nil, store.ErrInvalidCartQuantity
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO cart_lines (cart_id, variant_id, quantity, personalizations, personalization_hash)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (cart_id, variant_id, personalization_hash)
		DO UPDATE SET quantity = cart_lines.quantity + excluded.quantity
	`, cartID, variantID, quantity, snapshot, personalizationHash); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetCart(ctx, shopID, tokenHash)
}

func (s *Store) CheckoutCart(ctx context.Context, shopID string, tokenHash []byte, details store.CheckoutDetails) (*store.OrderView, error) {
	if len(tokenHash) != 32 {
		return nil, store.ErrInvalidCartToken
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.beginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var cartID, currency, locale string
	err = tx.QueryRowContext(ctx, `
		SELECT c.id, c.currency, s.default_locale
		FROM carts c JOIN shops s ON s.id = c.shop_id
		WHERE c.shop_id = ? AND c.access_token_hash = ? AND c.access_token_expires_at > ?
	`, shopID, tokenHash, time.Now().Unix()).Scan(&cartID, &currency, &locale)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrInvalidCartToken
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT v.id, v.sku, cl.quantity, COALESCE(pt.name, p.slug), pr.amount,
		       cl.personalizations, p.personalization_options
		FROM cart_lines cl
		JOIN variants v ON v.id = cl.variant_id
		JOIN products p ON p.id = v.product_id
		JOIN prices pr ON pr.variant_id = v.id AND pr.currency = ?
		LEFT JOIN product_translations pt ON pt.product_id = p.id AND pt.locale = ?
		WHERE cl.cart_id = ? AND p.shop_id = ? AND p.status = 'active'
		ORDER BY v.id, cl.personalization_hash
	`, currency, locale, cartID, shopID)
	if err != nil {
		return nil, err
	}
	lines := make([]checkoutLine, 0)
	subtotal := new(big.Rat)
	for rows.Next() {
		var line checkoutLine
		var personalizationsJSON, optionJSON []byte
		if err := rows.Scan(
			&line.variantID, &line.SKU, &line.Quantity, &line.Name, &line.UnitPrice,
			&personalizationsJSON, &optionJSON,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var options []store.PersonalizationOption
		if err := json.Unmarshal(optionJSON, &options); err != nil {
			_ = rows.Close()
			return nil, err
		}
		var snapshot []store.PersonalizationSelection
		if err := json.Unmarshal(personalizationsJSON, &snapshot); err != nil {
			_ = rows.Close()
			return nil, err
		}
		selections := make(map[string]string, len(snapshot))
		for _, selection := range snapshot {
			selections[selection.Code] = selection.Value
		}
		line.Personalizations, _, _, err = store.ValidatePersonalizationSelections(options, selections)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		unitPrice, ok := new(big.Rat).SetString(line.UnitPrice)
		if !ok {
			_ = rows.Close()
			return nil, errors.New("invalid product price")
		}
		lineAmount := new(big.Rat).Mul(unitPrice, big.NewRat(int64(line.Quantity), 1))
		line.LineTotal = lineAmount.FloatString(4)
		subtotal.Add(subtotal, lineAmount)
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, store.ErrEmptyCart
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM cart_lines WHERE cart_id = ?`, cartID).Scan(&count); err != nil {
		return nil, err
	}
	if count != len(lines) {
		return nil, store.ErrNotFound
	}
	quantities := make(map[string]int, len(lines))
	for _, line := range lines {
		quantities[line.variantID] += line.Quantity
	}
	for variantID, quantity := range quantities {
		result, err := tx.ExecContext(ctx, `
			UPDATE inventory_items SET quantity = quantity - ?
			WHERE variant_id = ? AND track = 1 AND quantity >= ?
		`, quantity, variantID, quantity)
		if err != nil {
			return nil, err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if updated == 0 {
			var tracked int
			err := tx.QueryRowContext(ctx, `SELECT track FROM inventory_items WHERE variant_id = ?`, variantID).Scan(&tracked)
			if errors.Is(err, sql.ErrNoRows) || tracked != 0 {
				return nil, store.ErrOutOfStock
			}
			if err != nil {
				return nil, err
			}
		}
	}

	customerID, err := s.upsertCustomer(ctx, tx, shopID, details.Email, details.Name)
	if err != nil {
		return nil, err
	}
	orderID, orderNumber, err := newOrderIdentifiers()
	if err != nil {
		return nil, err
	}
	addressJSON, err := json.Marshal(details.ShippingAddress)
	if err != nil {
		return nil, err
	}
	total := subtotal.FloatString(4)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO orders (
			id, shop_id, customer_id, number, status, currency, subtotal,
			tax_total, shipping_total, grand_total, shipping_address, placed_at
		) VALUES (?, ?, ?, ?, 'pending', ?, ?, '0.0000', '0.0000', ?, ?, ?)
	`, orderID, shopID, customerID, orderNumber, currency, total, total, string(addressJSON), time.Now().Unix()); err != nil {
		return nil, err
	}
	for _, line := range lines {
		personalizationJSON, err := json.Marshal(line.Personalizations)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO order_lines (
				order_id, variant_id, sku, name, quantity, unit_price, line_total, personalization_snapshot
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, orderID, line.variantID, line.SKU, line.Name, line.Quantity, line.UnitPrice, line.LineTotal, string(personalizationJSON)); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cart_lines WHERE cart_id = ?`, cartID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &store.OrderView{
		ID: orderID, Number: orderNumber, Status: "pending", Currency: currency,
		Subtotal: total, TaxTotal: "0.0000", ShippingTotal: "0.0000", GrandTotal: total,
		Items: orderLineViews(lines),
	}, nil
}

type checkoutLine struct {
	variantID string
	store.OrderLineView
}

func (s *Store) upsertCustomer(ctx context.Context, tx *sql.Tx, shopID, email, name string) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	email = strings.TrimSpace(email)
	emailKey := strings.ToLower(email)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO customers (id, shop_id, email, email_key, name)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (shop_id, email_key) DO UPDATE SET name = excluded.name
	`, id, shopID, email, emailKey, name); err != nil {
		return "", err
	}
	var customerID string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM customers WHERE shop_id = ? AND email_key = ?
	`, shopID, emailKey).Scan(&customerID)
	return customerID, err
}

func orderLineViews(lines []checkoutLine) []store.OrderLineView {
	items := make([]store.OrderLineView, len(lines))
	for i := range lines {
		items[i] = lines[i].OrderLineView
	}
	return items
}

func newOrderIdentifiers() (string, string, error) {
	id, err := newUUID()
	if err != nil {
		return "", "", err
	}
	return id, "EU-" + strings.ToUpper(strings.ReplaceAll(id, "-", "")[:20]), nil
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = raw[6]&0x0f | 0x40
	raw[8] = raw[8]&0x3f | 0x80
	encoded := hex.EncodeToString(raw[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
