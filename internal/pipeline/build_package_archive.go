package pipeline

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

const (
	MaxBuildPackageArchiveBytes = 64 << 20
	maxBuildPackageManifestSize = 64 << 10
	maxBuildPackageIRSize       = 1 << 20
	maxBuildPackageWASMSize     = 64 << 20
)

type SignedBuildPackageArchive struct {
	Signature     BuildPackageSignature
	SignatureJSON []byte
	ManifestJSON  []byte
	IRJSON        []byte
	WASMBytes     []byte
}

// ReadSignedBuildPackageArchive reads the exact supported ZIP transport without extracting files.
func ReadSignedBuildPackageArchive(archive []byte) (SignedBuildPackageArchive, error) {
	if len(archive) == 0 || len(archive) > MaxBuildPackageArchiveBytes {
		return SignedBuildPackageArchive{}, errors.New("build package archive has an invalid size")
	}
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return SignedBuildPackageArchive{}, fmt.Errorf("read build package ZIP: %w", err)
	}
	if len(reader.File) != 4 {
		return SignedBuildPackageArchive{}, fmt.Errorf("build package ZIP contains %d entries, want exactly 4", len(reader.File))
	}

	result := SignedBuildPackageArchive{}
	found := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		maxSize, ok := buildPackageEntryLimit(file.Name)
		if !ok || found[file.Name] {
			return SignedBuildPackageArchive{}, fmt.Errorf("build package ZIP contains unexpected or duplicate entry %q", file.Name)
		}
		found[file.Name] = true
		if file.Mode()&os.ModeType != 0 || file.FileInfo().IsDir() {
			return SignedBuildPackageArchive{}, fmt.Errorf("build package ZIP entry %q is not a regular file", file.Name)
		}
		if file.Method != zip.Store && file.Method != zip.Deflate {
			return SignedBuildPackageArchive{}, fmt.Errorf("build package ZIP entry %q uses an unsupported compression method", file.Name)
		}
		if file.UncompressedSize64 > uint64(maxSize) || file.CompressedSize64 > MaxBuildPackageArchiveBytes {
			return SignedBuildPackageArchive{}, fmt.Errorf("build package ZIP entry %q exceeds its size limit", file.Name)
		}
		data, err := readBuildPackageEntry(file, maxSize)
		if err != nil {
			return SignedBuildPackageArchive{}, fmt.Errorf("read build package ZIP entry %q: %w", file.Name, err)
		}
		switch file.Name {
		case "signature.json":
			result.SignatureJSON = data
		case "manifest.json":
			result.ManifestJSON = data
		case "module.ir.json":
			result.IRJSON = data
		case "module.wasm":
			result.WASMBytes = data
		}
	}
	for _, name := range []string{"signature.json", "manifest.json", "module.ir.json", "module.wasm"} {
		if !found[name] {
			return SignedBuildPackageArchive{}, fmt.Errorf("build package ZIP is missing %q", name)
		}
	}
	if err := decodeStrictJSON(result.SignatureJSON, &result.Signature); err != nil {
		return SignedBuildPackageArchive{}, fmt.Errorf("decode build package ZIP signature: %w", err)
	}
	return result, nil
}

func buildPackageEntryLimit(name string) (int64, bool) {
	switch name {
	case "signature.json":
		return maxBuildPackageSignatureEnvelopeBytes, true
	case "manifest.json":
		return maxBuildPackageManifestSize, true
	case "module.ir.json":
		return maxBuildPackageIRSize, true
	case "module.wasm":
		return maxBuildPackageWASMSize, true
	default:
		return 0, false
	}
}

func readBuildPackageEntry(file *zip.File, maxSize int64) ([]byte, error) {
	entry, err := file.Open()
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(entry, maxSize+1))
	closeErr := entry.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > maxSize || uint64(len(data)) != file.UncompressedSize64 {
		return nil, errors.New("entry size does not match its declared size or exceeds its limit")
	}
	return data, nil
}
