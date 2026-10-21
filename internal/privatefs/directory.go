package privatefs

// CreateTempDirectory creates a uniquely named directory with private access.
func CreateTempDirectory(parent, prefix string) (string, error) {
	return createTempDirectory(parent, prefix)
}

// EnsureDirectory creates path if needed and rejects directories with broader access.
func EnsureDirectory(path string) error {
	return ensurePrivateDirectory(path)
}
