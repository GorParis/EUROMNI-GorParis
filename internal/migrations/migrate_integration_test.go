//go:build integration

package migrations_test

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"

	migrationrunner "euromni/internal/migrations"
	"euromni/internal/testdb"
	schemafiles "euromni/migrations"
)

func TestApplyIsIdempotentAndFailedMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	db := testdb.Start(t)

	applied, err := migrationrunner.Apply(ctx, db, schemafiles.Files)
	if err != nil {
		t.Fatalf("apply already-installed migrations: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("repeated migration run applied %v, want no changes", applied)
	}

	source := fstest.MapFS{}
	if err := fs.WalkDir(schemafiles.Files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := fs.ReadFile(schemafiles.Files, path)
		if err != nil {
			return err
		}
		source[path] = &fstest.MapFile{Data: data}
		return nil
	}); err != nil {
		t.Fatalf("copy embedded migrations: %v", err)
	}
	source["0008_test_failure.sql"] = &fstest.MapFile{Data: []byte(`
		CREATE TABLE migration_rollback_probe (id integer);
		SELECT nonexistent_migration_function();
	`)}
	if _, err := migrationrunner.Apply(ctx, db, source); err == nil {
		t.Fatal("apply accepted a migration containing invalid SQL")
	}
	var probeExists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.migration_rollback_probe') IS NOT NULL`).Scan(&probeExists); err != nil {
		t.Fatalf("check rollback probe: %v", err)
	}
	if probeExists {
		t.Fatal("failed migration left its table behind")
	}
	var ledgerContainsFailedMigration bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '0008_test_failure.sql')`).Scan(&ledgerContainsFailedMigration); err != nil {
		t.Fatalf("check failed migration ledger entry: %v", err)
	}
	if ledgerContainsFailedMigration {
		t.Fatal("failed migration was recorded in the ledger")
	}

	if _, err := db.ExecContext(ctx, `CREATE TABLE schema_migrations_existing_schema_probe (id integer)`); err != nil {
		t.Fatalf("verify connection remains usable after rollback: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ('9999_unknown.sql')`); err != nil {
		t.Fatalf("insert unknown ledger version: %v", err)
	}
	if _, err := migrationrunner.Apply(ctx, db, schemafiles.Files); err == nil {
		t.Fatal("apply accepted an unknown migration ledger version")
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = '9999_unknown.sql'; DROP TABLE schema_migrations`); err != nil {
		t.Fatalf("prepare unmanaged-schema check: %v", err)
	}
	if _, err := migrationrunner.Apply(ctx, db, schemafiles.Files); err == nil {
		t.Fatal("apply accepted an EUROMNI schema without a migration ledger")
	}
}
