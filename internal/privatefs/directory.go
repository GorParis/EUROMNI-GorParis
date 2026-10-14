package privatefs

// EnsureDirectory creates path if needed and rejects directories with broader access.
func EnsureDirectory(path string) error {
	return ensurePrivateDirectory(path)
}
