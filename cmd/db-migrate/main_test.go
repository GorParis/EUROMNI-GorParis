package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("run() error = %v, want DATABASE_URL required", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run() wrote success output without a database: %q", stdout.String())
	}
}

func TestRunRejectsUnexpectedArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), []string{"unexpected"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "unexpected positional arguments") {
		t.Fatalf("run() error = %v, want unexpected arguments", err)
	}
}

func TestRunRejectsInvalidDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), nil, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "parse DATABASE_URL") {
		t.Fatalf("run() error = %v, want DATABASE_URL parsing error", err)
	}
}
