//go:build integration

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"euromni/internal/privatefs"
	"euromni/internal/store"
	"euromni/internal/testdb"
)

func TestPurgeUsesPostgreSQLCandidateCoordinator(t *testing.T) {
	database := store.NewSQL(testdb.Start(t))
	root := filepath.Join(t.TempDir(), "candidates")
	if err := privatefs.EnsureDirectory(root); err != nil {
		t.Fatalf("create candidate directory: %v", err)
	}
	var output bytes.Buffer
	if err := purge(root, database, &output); err != nil {
		t.Fatalf("purge() error = %v", err)
	}
	if !strings.Contains(output.String(), "removed=0") {
		t.Fatalf("purge output = %q, want no removals", output.String())
	}
}
