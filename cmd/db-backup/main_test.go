package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRequiresDatabaseURLForBackup(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("EUROMNI_ARTIFACT_DIR", t.TempDir())
	t.Setenv("EUROMNI_BACKUP_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	err := run(context.Background(), nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("run() error = %v, want DATABASE_URL required", err)
	}
}

func TestRunVerifyReportsMissingBackup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"-verify", t.TempDir() + "-missing"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "inspect backup directory") {
		t.Fatalf("run() error = %v, want backup inspection error", err)
	}
}

func TestRunRestoreRequiresDestination(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"-restore-artifacts", t.TempDir()}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "-artifact-dir is required") {
		t.Fatalf("run() error = %v, want artifact restore destination required", err)
	}
}

func TestRunRejectsPositionalArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"unexpected"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("run() error = %v, want positional argument error", err)
	}
}
