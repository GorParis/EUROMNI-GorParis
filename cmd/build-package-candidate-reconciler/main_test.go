package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresPostgreSQLCoordinator(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := run([]string{"-dir", root}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("run() error = %v, want PostgreSQL coordinator requirement", err)
	}
}

func TestRunRequiresExistingCandidateDirectory(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	existingDirectory := t.TempDir()
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "directory",
			args: nil,
			want: "-dir or EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR is required",
		},
		{
			name: "missing directory",
			args: []string{"-dir", filepath.Join(t.TempDir(), "missing")},
			want: "inspect build package candidate directory",
		},
		{
			name: "unexpected argument",
			args: []string{"-dir", existingDirectory, "extra"},
			want: "unexpected positional arguments",
		},
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
