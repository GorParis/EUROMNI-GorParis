package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"euromni/internal/packagecandidate"
)

func TestRunReportsRetentionAndLeavesEmptyStoreIntact(t *testing.T) {
	root := filepath.Join(t.TempDir(), "candidates")
	if _, err := packagecandidate.NewFileStore(root); err != nil {
		t.Fatalf("create candidate store: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := run([]string{"-dir", root}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v; stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "removed=0") ||
		!strings.Contains(stdout.String(), packagecandidate.RejectedCandidateRetention.String()) {
		t.Fatalf("run() output = %q, want retention and no removals", stdout.String())
	}
}

func TestRunRequiresExistingCandidateDirectory(t *testing.T) {
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
			args: []string{"-dir", t.TempDir(), "extra"},
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
