package artifact

import "euromni/internal/privatefs"

func ensurePrivateDirectory(path string) error {
	return privatefs.EnsureDirectory(path)
}
