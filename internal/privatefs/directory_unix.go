//go:build !windows

package privatefs

import (
	"errors"
	"fmt"
	"os"
)

func createTempDirectory(parent, prefix string) (string, error) {
	if parent == "" {
		parent = os.TempDir()
	}
	return os.MkdirTemp(parent, prefix)
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create private directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return errors.New("directory must be private")
	}
	return nil
}
