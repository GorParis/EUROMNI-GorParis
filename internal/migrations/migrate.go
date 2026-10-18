package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"

	"euromni/internal/dbdialect"
)

const advisoryLockID int64 = 714253896214

var migrationNamePattern = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)

type migration struct {
	name string
}

// Apply runs pending PostgreSQL migrations from source. The connection must
// support PostgreSQL simple-protocol execution because migrations are SQL
// scripts containing multiple statements and their own transaction boundaries.
func Apply(ctx context.Context, db *sql.DB, source fs.FS) (applied []string, retErr error) {
	if db == nil {
		return nil, errors.New("migration database is not configured")
	}

	ordered, err := list(source)
	if err != nil {
		return nil, err
	}
	if len(ordered) < 4 {
		return nil, errors.New("migration source must contain the four baseline migrations")
	}
	for i, name := range []string{
		"0001_init.sql",
		"0002_seed_example.sql",
		"0003_cart_access_tokens.sql",
		"0004_schema_migrations.sql",
	} {
		if ordered[i].name != name {
			return nil, fmt.Errorf("baseline migration %d must be %s, got %s", i+1, name, ordered[i].name)
		}
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return nil, fmt.Errorf("lock schema migrations: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, advisoryLockID); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("unlock schema migrations: %w", err))
		}
	}()

	applied = make([]string, 0)
	hasLedger, err := relationExists(ctx, conn, "schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("inspect migration ledger: %w", err)
	}
	if !hasLedger {
		hasSchema, err := relationExists(ctx, conn, "shops")
		if err != nil {
			return nil, fmt.Errorf("inspect existing schema: %w", err)
		}
		if hasSchema {
			return nil, errors.New("database has an EUROMNI schema without schema_migrations; refusing to replay baseline migrations")
		}
		if err := applyBaseline(ctx, conn, source, ordered[:4]); err != nil {
			return nil, err
		}
		applied = append(applied, ordered[0].name, ordered[1].name, ordered[2].name, ordered[3].name)
	}

	recorded, err := readLedger(ctx, conn)
	if err != nil {
		return nil, err
	}
	for i, item := range ordered {
		if i < len(recorded) {
			if recorded[i] != item.name {
				return nil, fmt.Errorf("migration ledger is not a known contiguous prefix: entry %d is %q, expected %q", i+1, recorded[i], item.name)
			}
			continue
		}
		if err := execute(ctx, conn, source, item.name); err != nil {
			return nil, err
		}
		hasVersion, err := migrationRecorded(ctx, conn, item.name)
		if err != nil {
			return nil, fmt.Errorf("verify migration %s ledger entry: %w", item.name, err)
		}
		if !hasVersion {
			return nil, fmt.Errorf("migration %s completed without recording its version in schema_migrations", item.name)
		}
		applied = append(applied, item.name)
		recorded = append(recorded, item.name)
	}
	if len(recorded) > len(ordered) {
		return nil, fmt.Errorf("migration ledger contains %d entries but only %d migration files are available", len(recorded), len(ordered))
	}
	return applied, nil
}

func ApplyWithDialect(ctx context.Context, db *sql.DB, source fs.FS, dialect dbdialect.Dialect) ([]string, error) {
	switch dialect {
	case dbdialect.MariaDB:
		return ApplyMariaDB(ctx, db, source)
	case dbdialect.PostgreSQL:
		return Apply(ctx, db, source)
	default:
		return nil, fmt.Errorf("unsupported database dialect %q", dialect)
	}
}

func ApplyMariaDB(ctx context.Context, db *sql.DB, source fs.FS) (applied []string, retErr error) {
	if db == nil {
		return nil, errors.New("migration database is not configured")
	}
	ordered, err := list(source)
	if err != nil {
		return nil, err
	}
	if ordered[0].name != "0001_mariadb_schema.sql" {
		return nil, errors.New("MariaDB migration source must start with 0001_mariadb_schema.sql")
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire MariaDB migration connection: %w", err)
	}
	defer conn.Close()
	var locked int
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK($1, 10)`, "euromni:schema-migrations").Scan(&locked); err != nil {
		return nil, fmt.Errorf("lock MariaDB schema migrations: %w", err)
	}
	if locked != 1 {
		return nil, errors.New("timed out waiting for MariaDB schema migration lock")
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var released int
		if err := conn.QueryRowContext(unlockCtx, `SELECT RELEASE_LOCK($1)`, "euromni:schema-migrations").Scan(&released); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("unlock MariaDB schema migrations: %w", err))
		} else if released != 1 {
			retErr = errors.Join(retErr, errors.New("MariaDB schema migration lock was not released"))
		}
	}()

	hasLedger, err := mariaDBTableExists(ctx, conn, "schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("inspect MariaDB migration ledger: %w", err)
	}
	if !hasLedger {
		hasSchema, err := mariaDBTableExists(ctx, conn, "shops")
		if err != nil {
			return nil, fmt.Errorf("inspect existing MariaDB schema: %w", err)
		}
		if hasSchema {
			return nil, errors.New("database has an EUROMNI schema without schema_migrations; refusing to replay MariaDB baseline")
		}
		if err := executeMariaDB(ctx, conn, source, ordered[0].name, false); err != nil {
			return nil, err
		}
		recorded, err := mariaDBMigrationRecorded(ctx, conn, ordered[0].name)
		if err != nil {
			return nil, fmt.Errorf("verify MariaDB baseline ledger entry: %w", err)
		}
		if !recorded {
			return nil, errors.New("MariaDB baseline completed without recording its version in schema_migrations")
		}
		applied = append(applied, ordered[0].name)
	}

	recorded, err := readLedger(ctx, conn)
	if err != nil {
		return nil, err
	}
	if len(recorded) == 0 {
		return nil, errors.New("MariaDB migration ledger exists without its baseline entry")
	}
	if len(recorded) > len(ordered) {
		return nil, fmt.Errorf("MariaDB migration ledger contains %d entries but only %d migration files are available", len(recorded), len(ordered))
	}
	for i, version := range recorded {
		if ordered[i].name != version {
			return nil, fmt.Errorf("MariaDB migration ledger is not a known contiguous prefix: entry %d is %q, expected %q", i+1, version, ordered[i].name)
		}
	}
	for i := len(recorded); i < len(ordered); i++ {
		name := ordered[i].name
		if err := executeMariaDB(ctx, conn, source, name, true); err != nil {
			return nil, err
		}
		recordedVersion, err := mariaDBMigrationRecorded(ctx, conn, name)
		if err != nil {
			return nil, fmt.Errorf("verify MariaDB migration %s ledger entry: %w", name, err)
		}
		if !recordedVersion {
			return nil, fmt.Errorf("MariaDB migration %s completed without recording its version in schema_migrations", name)
		}
		applied = append(applied, name)
	}
	return applied, nil
}

func mariaDBTableExists(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
	var exists bool
	err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_name = $1
		)
	`, name).Scan(&exists)
	return exists, err
}

func mariaDBMigrationRecorded(ctx context.Context, conn *sql.Conn, version string) (bool, error) {
	var recorded bool
	err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)
	`, version).Scan(&recorded)
	return recorded, err
}

func executeMariaDB(ctx context.Context, conn *sql.Conn, source fs.FS, name string, record bool) error {
	script, err := fs.ReadFile(source, name)
	if err != nil {
		return fmt.Errorf("read MariaDB migration %s: %w", name, err)
	}
	if !record {
		if _, err := conn.ExecContext(ctx, string(script)); err != nil {
			return fmt.Errorf("apply MariaDB migration %s (MariaDB DDL may have committed earlier statements): %w", name, err)
		}
		return nil
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin MariaDB migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, string(script)); err != nil {
		return rollback(tx, fmt.Errorf("apply MariaDB migration %s (MariaDB DDL may have committed earlier statements): %w", name, err))
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
		return rollback(tx, fmt.Errorf("record MariaDB migration %s: %w", name, err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit MariaDB migration %s: %w", name, err)
	}
	return nil
}

func list(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return nil, fmt.Errorf("read migration source: %w", err)
	}
	ordered := make([]migration, 0, len(entries))
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		name := entry.Name()
		if !migrationNamePattern.MatchString(name) {
			return nil, fmt.Errorf("invalid migration filename %q; expected NNNN_description.sql", name)
		}
		version := name[:4]
		if previous, exists := versions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %s in %s and %s", version, previous, name)
		}
		versions[version] = name
		ordered = append(ordered, migration{name: name})
	}
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].name < ordered[j].name
	})
	if len(ordered) == 0 {
		return nil, errors.New("migration source contains no SQL migrations")
	}
	return ordered, nil
}

func relationExists(ctx context.Context, conn *sql.Conn, name string) (bool, error) {
	var exists bool
	err := conn.QueryRowContext(ctx, `SELECT to_regclass('public.' || $1) IS NOT NULL`, name).Scan(&exists)
	return exists, err
}

func readLedger(ctx context.Context, conn *sql.Conn) ([]string, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	defer rows.Close()
	recorded := make([]string, 0)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration ledger: %w", err)
		}
		recorded = append(recorded, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read migration ledger rows: %w", err)
	}
	return recorded, nil
}

func migrationRecorded(ctx context.Context, conn *sql.Conn, version string) (bool, error) {
	var recorded bool
	err := conn.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&recorded)
	return recorded, err
}

func execute(ctx context.Context, conn *sql.Conn, source fs.FS, name string) error {
	script, err := fs.ReadFile(source, name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, string(script)); err != nil {
		return rollback(tx, fmt.Errorf("apply migration %s: %w", name, err))
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
		return rollback(tx, fmt.Errorf("record migration %s: %w", name, err))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

func applyBaseline(ctx context.Context, conn *sql.Conn, source fs.FS, baseline []migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin baseline migrations: %w", err)
	}
	for _, item := range baseline {
		script, err := fs.ReadFile(source, item.name)
		if err != nil {
			return rollback(tx, fmt.Errorf("read migration %s: %w", item.name, err))
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			return rollback(tx, fmt.Errorf("apply migration %s: %w", item.name, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit baseline migrations: %w", err)
	}
	return nil
}

func rollback(tx *sql.Tx, cause error) error {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		return errors.Join(cause, fmt.Errorf("rollback migration transaction: %w", err))
	}
	return cause
}
