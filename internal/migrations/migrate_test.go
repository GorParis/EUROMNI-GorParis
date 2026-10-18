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
	if len(ordered) != 4 || ordered[0].name != "0001_mariadb_schema.sql" ||
		ordered[1].name != "0002_mariadb_demo_seed.sql" || ordered[2].name != "0003_shipping_weight_value_rates.sql" ||
		ordered[3].name != "0004_shipping_rate_mode_check.sql" {
		t.Fatalf("unexpected MariaDB migration order: %#v", ordered)
	}
	migration, err := schemafiles.MariaDBFiles.ReadFile("mariadb/0004_shipping_rate_mode_check.sql")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migration), "CHECK (rate_mode IN ('fixed', 'weight_value'))") {
		t.Fatal("MariaDB rate mode migration does not constrain supported modes")
	}
}
