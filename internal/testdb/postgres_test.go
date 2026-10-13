//go:build integration

package testdb

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectRootContainsMigrations(t *testing.T) {
	root := projectRoot(t)
	if _, err := os.Stat(filepath.Join(root, "migrations", "0006_seed_representative_catalog.sql")); err != nil {
		t.Fatalf("project migration ledger is not reachable from %s: %v", root, err)
	}
}
