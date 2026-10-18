package dbdialect

import (
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"
)

func TestRewriteMariaDBQuery(t *testing.T) {
	query := `
		SELECT id::text, amount::numeric(19, 4)::text
		FROM products
		WHERE shop_id = $2 AND note = '$3' AND id = $1
		  AND countries @> jsonb_build_array($3::text)
	`
	rewritten, order, err := rewriteMariaDBQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rewritten, "::") || strings.Contains(rewritten, "@>") || strings.Contains(rewritten, "jsonb_build_array") {
		t.Fatalf("PostgreSQL syntax remained in rewritten SQL: %s", rewritten)
	}
	if !strings.Contains(rewritten, "JSON_CONTAINS(countries, JSON_QUOTE(?))") {
		t.Fatalf("JSON membership was not rewritten: %s", rewritten)
	}
	if !strings.Contains(rewritten, "note = '$3'") {
		t.Fatalf("placeholder inside a string literal was rewritten: %s", rewritten)
	}
	if !reflect.DeepEqual(order, []int{1, 0, 2}) {
		t.Fatalf("placeholder order = %v, want [1 0 2]", order)
	}
}

func TestRewriteMariaDBUpsertAndLock(t *testing.T) {
	query := `INSERT INTO inventory_items (variant_id, quantity) VALUES ($1, $2)
		ON CONFLICT (variant_id) DO UPDATE SET quantity = EXCLUDED.quantity
		SELECT id FROM variants FOR UPDATE OF variants`
	rewritten, _, err := rewriteMariaDBQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, "ON DUPLICATE KEY UPDATE quantity = VALUES(quantity)") {
		t.Fatalf("upsert was not rewritten: %s", rewritten)
	}
	if !strings.Contains(rewritten, "FOR UPDATE") || strings.Contains(rewritten, "FOR UPDATE OF") {
		t.Fatalf("locking clause was not rewritten: %s", rewritten)
	}
}

func TestReorderNamedValues(t *testing.T) {
	input := []driver.NamedValue{
		{Ordinal: 1, Value: "first"},
		{Ordinal: 2, Value: "second"},
	}
	got, err := reorderNamedValues(input, []int{1, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	want := []driver.NamedValue{
		{Ordinal: 1, Value: "second"},
		{Ordinal: 2, Value: "first"},
		{Ordinal: 3, Value: "second"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered args = %#v, want %#v", got, want)
	}
}

func TestMariaDBStatementInputCountUsesNumberedArguments(t *testing.T) {
	statement := &mariaDBStmt{order: []int{0, 0, 2}}
	if got := statement.NumInput(); got != 3 {
		t.Fatalf("NumInput() = %d, want 3 for $1, $1, $3", got)
	}
}

func TestRewriteMariaDBRejectsReturningAndMissingArguments(t *testing.T) {
	if _, _, err := rewriteMariaDBQuery(`INSERT INTO products (id) VALUES ($1) RETURNING id`); err == nil {
		t.Fatal("PostgreSQL RETURNING was accepted")
	}
	if _, err := reorderNamedValues([]driver.NamedValue{{Ordinal: 1, Value: "only"}}, []int{1}); err == nil {
		t.Fatal("missing placeholder argument was accepted")
	}
}
