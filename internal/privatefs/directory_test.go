package privatefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDirectoryCreatesAndValidatesPrivateDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database")
	if err := EnsureDirectory(path); err != nil {
		t.Fatalf("create private directory: %v", err)
	}
	if err := EnsureDirectory(path); err != nil {
		t.Fatalf("validate existing private directory: %v", err)
	}
}

func TestCreateTempDirectoryCreatesPrivateDirectory(t *testing.T) {
	path, err := CreateTempDirectory(t.TempDir(), "private-")
	if err != nil {
		t.Fatalf("create private temporary directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Errorf("remove private temporary directory: %v", err)
		}
	})
	if err := EnsureDirectory(path); err != nil {
		t.Fatalf("validate private temporary directory: %v", err)
	}
}

func TestEnsureDirectoryRejectsSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := EnsureDirectory(link); err == nil {
		t.Fatal("EnsureDirectory accepted a symlink")
	}
}
