package migrations

import (
	"strings"
	"testing"
)

func TestMariaDBSchemaIsEmbedded(t *testing.T) {
	schema, err := MariaDBFiles.ReadFile("mariadb/0001_mariadb_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"CREATE TABLE shops",
		"CREATE TABLE shipping_carriers",
		"CREATE TABLE shipping_zones",
		"CREATE TABLE shipping_rates",
		"CREATE TABLE schema_migrations",
		"CREATE TRIGGER trg_shops_updated_at",
	} {
		if !strings.Contains(string(schema), expected) {
			t.Errorf("MariaDB schema is missing %q", expected)
		}
	}
	for _, postgresOnly := range []string{
		"::jsonb",
		"gen_random_uuid()",
		"timestamptz",
		"citext",
		"pg_advisory_lock",
	} {
		if strings.Contains(strings.ToLower(string(schema)), strings.ToLower(postgresOnly)) {
			t.Errorf("MariaDB schema contains PostgreSQL-specific syntax %q", postgresOnly)
		}
	}
}
