package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"euromni/internal/quarantine"
)

func TestRunPurgesArchivesUsingExplicitAge(t *testing.T) {
	root := filepath.Join(t.TempDir(), "quarantine")
	store, err := quarantine.NewFileStore(root)
	if err != nil {
		t.Fatalf("create quarantine store: %v", err)
	}
	archive, err := store.Put(context.Background(), "shop", []byte("expired archive"))
	if err != nil {
		t.Fatalf("store archive: %v", err)
	}
	archivePaths, err := filepath.Glob(filepath.Join(root, "*", archive.SHA256))
	if err != nil || len(archivePaths) != 1 {
		t.Fatalf("find stored archive: paths=%v error=%v", archivePaths, err)
	}
	archivePath := archivePaths[0]
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(archivePath, oldTime, oldTime); err != nil {
		t.Fatalf("age archive: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := run([]string{"-dir", root, "-older-than=24h"}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "removed=1") {
		t.Fatalf("run() output = %q, want one removal", stdout.String())
	}
}

func TestRunRequiresPositiveAgeAndQuarantineDirectory(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "directory", args: []string{"-older-than=24h"}, want: "-dir or EUROMNI_QUARANTINE_DIR is required"},
		{name: "age", args: []string{"-dir", t.TempDir()}, want: "-older-than must be a positive Go duration"},
		{name: "zero age", args: []string{"-dir", t.TempDir(), "-older-than=0s"}, want: "-older-than must be a positive Go duration"},
		{name: "missing directory", args: []string{"-dir", filepath.Join(t.TempDir(), "missing"), "-older-than=24h"}, want: "inspect quarantine directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(test.args, &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("run() error = %v, want %q", err, test.want)
			}
		})
	}
}