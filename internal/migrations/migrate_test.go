package migrations

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	schemafiles "euromni/migrations"
)

func TestListRejectsDuplicateMigrationVersion(t *testing.T) {
	source := fstest.MapFS{
		"0001_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
		"0001_second.sql":  &fstest.MapFile{Data: []byte("SELECT 2;")},
	}
	if _, err := list(source); err == nil {
		t.Fatal("list() accepted duplicate migration versions")
	}
}

func TestListRejectsInvalidMigrationFilename(t *testing.T) {
	source := fstest.MapFS{
		"1_initial.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}
	if _, err := list(source); err == nil {
		t.Fatal("list() accepted an invalid migration filename")
	}
}

func TestMariaDBMigrationSourceStartsWithInstalledSchema(t *testing.T) {
	source, err := fs.Sub(schemafiles.MariaDBFiles, "mariadb")
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := list(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 10 || ordered[0].name != "0001_mariadb_schema.sql" ||
		ordered[1].name != "0002_mariadb_demo_seed.sql" || ordered[2].name != "0003_shipping_weight_value_rates.sql" ||
		ordered[3].name != "0004_shipping_rate_mode_check.sql" ||
		ordered[4].name != "0005_shipping_services_rate_versions.sql" ||
		ordered[5].name != "0006_catalog_references_personalization.sql" ||
		ordered[6].name != "0007_shipping_policy_snapshot.sql" ||
		ordered[7].name != "0008_payment_event_idempotency.sql" ||
		ordered[8].name != "0009_supplier_order_sync.sql" ||
		ordered[9].name != "0010_immutable_module_builds.sql" {
		t.Fatalf("unexpected MariaDB migration order: %#v", ordered)
	}
	rateModeMigration, err := schemafiles.MariaDBFiles.ReadFile("mariadb/0004_shipping_rate_mode_check.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rateModeMigration), "CHECK (rate_mode IN ('fixed', 'weight_value'))") {
		t.Fatal("MariaDB rate mode migration does not constrain supported modes")
	}
	migration, err := schemafiles.MariaDBFiles.ReadFile("mariadb/0005_shipping_services_rate_versions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migration), "CREATE TABLE shipping_services") ||
		!strings.Contains(string(migration), "rate_version CHAR(36)") ||
		!strings.Contains(string(migration), "CREATE TABLE shipping_rate_versions") ||
		!strings.Contains(string(migration), "CREATE TABLE shipping_rate_version_rules") {
		t.Fatal("MariaDB shipping migration does not add services and immutable rate versions")
	}
	policySnapshotMigration, err := schemafiles.MariaDBFiles.ReadFile("mariadb/0007_shipping_policy_snapshot.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(policySnapshotMigration), "shipping_policy_snapshot") {
		t.Fatal("MariaDB shipping policy migration does not add the order snapshot")
	}
	paymentMigration, err := schemafiles.MariaDBFiles.ReadFile("mariadb/0008_payment_event_idempotency.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(paymentMigration), "provider_event_id") ||
		!strings.Contains(string(paymentMigration), "payments_provider_event_id_unique") {
		t.Fatal("MariaDB payment migration does not make provider events unique")
	}
	supplierSyncMigration, err := schemafiles.MariaDBFiles.ReadFile("mariadb/0009_supplier_order_sync.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(supplierSyncMigration), "supplier_snapshot") ||
		!strings.Contains(string(supplierSyncMigration), "supplier_order_syncs") ||
		!strings.Contains(string(supplierSyncMigration), "idempotency_key") {
		t.Fatal("MariaDB supplier sync migration does not add order snapshots and sync state")
	}
	buildMigration, err := schemafiles.MariaDBFiles.ReadFile("mariadb/0010_immutable_module_builds.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buildMigration), "CREATE TABLE module_builds") ||
		!strings.Contains(string(buildMigration), "current_build_id") ||
		!strings.Contains(string(buildMigration), "shop_modules_build") {
		t.Fatal("MariaDB build migration does not add immutable builds and installation pinning")
	}
}
