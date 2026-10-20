package main

import (
	"os"
	"strings"
	"testing"
)

func TestImportCommandDryRunUsesRealUpstreamArchive(t *testing.T) {
	if err := run([]string{
		"--archive", "../../pipeline/fixtures/prestashop-ps_newproducts.zip",
		"--platform-version", "euromni-test-adapter",
	}); err != nil {
		t.Fatalf("dry-run import: %v", err)
	}
}

func TestImportCommandRequiresExplicitDevelopmentInstall(t *testing.T) {
	t.Setenv("EUROMNI_ENVIRONMENT", "")
	err := run([]string{
		"--archive", "../../pipeline/fixtures/prestashop-ps_newproducts.zip",
		"--platform-version", "euromni-test-adapter",
		"--install",
	})
	if err == nil || !strings.Contains(err.Error(), "EUROMNI_ENVIRONMENT=development") {
		t.Fatalf("install without explicit development environment error = %v", err)
	}
}

func TestValidateTestDatabaseURL(t *testing.T) {
	for _, test := range []struct {
		name string
		dsn  string
		ok   bool
	}{
		{name: "compose database", dsn: "postgres://user:secret@postgres:5432/euromni?sslmode=disable", ok: true},
		{name: "local test database", dsn: "postgres://user:secret@127.0.0.1:5432/euromni", ok: true},
		{name: "remote host", dsn: "postgres://user:secret@db.example.test:5432/euromni"},
		{name: "non-loopback IP", dsn: "postgres://user:secret@192.0.2.10:5432/euromni"},
		{name: "wrong database", dsn: "postgres://user:secret@postgres:5432/production"},
		{name: "wrong dialect", dsn: "mysql://user:secret@localhost:3306/euromni"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateTestDatabaseURL(test.dsn)
			if (err == nil) != test.ok {
				t.Fatalf("validateTestDatabaseURL(%q) error = %v, want ok=%t", test.dsn, err, test.ok)
			}
		})
	}
}

func TestImportCommandRejectsSymlinkArchive(t *testing.T) {
	target := "../../pipeline/fixtures/prestashop-ps_newproducts.zip"
	link := t.TempDir() + "/package.zip"
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("create test symlink: %v", err)
	}
	err := run([]string{"--archive", link, "--platform-version", "test"})
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("symlink archive error = %v", err)
	}
}
