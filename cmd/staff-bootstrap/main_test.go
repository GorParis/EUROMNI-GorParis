package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsInvalidBootstrapArgumentsAndPasswords(t *testing.T) {
	validArgs := []string{
		"--shop", "shop-id",
		"--email", "owner@example.com",
		"--name", "Shop Owner",
		"--password-stdin",
	}
	tests := []struct {
		name     string
		args     []string
		password string
		wantErr  string
	}{
		{
			name:    "missing required flags",
			args:    []string{"--password-stdin"},
			wantErr: "required flags",
		},
		{
			name:    "unexpected positional argument",
			args:    append(append([]string(nil), validArgs...), "unexpected"),
			wantErr: "unexpected positional arguments",
		},
		{
			name:     "password shorter than minimum",
			args:     validArgs,
			password: "12345678901\n",
			wantErr:  "at least 12 bytes",
		},
		{
			name:     "password longer than bcrypt limit",
			args:     validArgs,
			password: strings.Repeat("x", 73),
			wantErr:  "at most 72 bytes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(test.args, strings.NewReader(test.password), &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("run error = %v, want error containing %q", err, test.wantErr)
			}
			if stdout.Len() != 0 {
				t.Fatalf("run printed success output for invalid input: %q", stdout.String())
			}
		})
	}
}

func TestRunAcceptsValidPasswordBeforeCheckingDatabaseConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	args := []string{
		"--shop", "shop-id",
		"--email", "owner@example.com",
		"--name", "Shop Owner",
		"--password-stdin",
	}
	var stdout, stderr bytes.Buffer
	err := run(args, strings.NewReader("123456789012\n"), &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("run error = %v, want missing DATABASE_URL after valid input", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("run printed success without a database: %q", stdout.String())
	}
}
