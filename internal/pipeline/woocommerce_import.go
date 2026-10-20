package pipeline

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	MaxWooCommerceArchiveBytes      = 16 << 20
	maxWooCommerceArchiveFiles      = 128
	maxWooCommerceEntryBytes        = 4 << 20
	maxWooCommerceTotalBytes        = 32 << 20
	wooCommerceAdapterVersion       = "1.0.0"
	wooCommerceSortingArchiveSHA256 = "65a239244a526bd98fb512a2ac5c5157379a9fcf8ffd2148c6efec007070144e"
)

var ErrUnsupportedWooCommercePackage = errors.New("unsupported WooCommerce package")

type WooCommercePackage struct {
	Name       string
	Version    string
	EntryPath  string
	EntryPoint []byte
	ArchiveSHA string
}

func ReadWooCommerceSortingArchive(data []byte) (WooCommercePackage, error) {
	if len(data) == 0 || len(data) > MaxWooCommerceArchiveBytes {
		return WooCommercePackage{}, fmt.Errorf("%w: archive size is invalid", ErrUnsupportedWooCommercePackage)
	}
	archiveDigest := sha256.Sum256(data)
	archiveSHA := hex.EncodeToString(archiveDigest[:])
	if archiveSHA != wooCommerceSortingArchiveSHA256 {
		return WooCommercePackage{}, fmt.Errorf("%w: archive hash is not the pinned source package", ErrUnsupportedWooCommercePackage)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > maxWooCommerceArchiveFiles {
		return WooCommercePackage{}, fmt.Errorf("%w: invalid ZIP archive", ErrUnsupportedWooCommercePackage)
	}

	const expectedRoot = "woocommerce-extra-product-sorting-options-bc5829d4d2383ae956869386eb40e772cf4a76a0"
	seen := make(map[string]struct{}, len(reader.File))
	var root string
	var entryPoint []byte
	var entryPath string
	var totalSize uint64
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if name == "" || strings.ContainsRune(name, '\x00') || strings.HasPrefix(name, "/") {
			return WooCommercePackage{}, fmt.Errorf("%w: unsafe archive path", ErrUnsupportedWooCommercePackage)
		}
		cleanName := strings.TrimSuffix(name, "/")
		cleaned := path.Clean(cleanName)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") ||
			strings.HasPrefix(cleaned, "/") || cleaned != cleanName || strings.Contains(cleaned, ":") {
			return WooCommercePackage{}, fmt.Errorf("%w: unsafe archive path %q", ErrUnsupportedWooCommercePackage, file.Name)
		}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 || mode.Type() != 0 && mode.Type() != os.ModeDir {
			return WooCommercePackage{}, fmt.Errorf("%w: symbolic links and special files are not allowed", ErrUnsupportedWooCommercePackage)
		}
		duplicateKey := strings.ToLower(cleaned)
		if _, exists := seen[duplicateKey]; exists {
			return WooCommercePackage{}, fmt.Errorf("%w: duplicate archive path %q", ErrUnsupportedWooCommercePackage, cleaned)
		}
		seen[duplicateKey] = struct{}{}

		parts := strings.Split(cleaned, "/")
		if root == "" {
			root = parts[0]
		} else if root != parts[0] {
			return WooCommercePackage{}, fmt.Errorf("%w: package has multiple root directories", ErrUnsupportedWooCommercePackage)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if len(parts) < 2 {
			return WooCommercePackage{}, fmt.Errorf("%w: package must have one root directory", ErrUnsupportedWooCommercePackage)
		}
		if file.UncompressedSize64 > maxWooCommerceEntryBytes ||
			totalSize > maxWooCommerceTotalBytes-file.UncompressedSize64 {
			return WooCommercePackage{}, fmt.Errorf("%w: archive expands beyond size limits", ErrUnsupportedWooCommercePackage)
		}
		totalSize += file.UncompressedSize64
		opened, err := file.Open()
		if err != nil {
			return WooCommercePackage{}, fmt.Errorf("%w: open archive entry: %v", ErrUnsupportedWooCommercePackage, err)
		}
		content, readErr := io.ReadAll(io.LimitReader(opened, maxWooCommerceEntryBytes+1))
		closeErr := opened.Close()
		if readErr != nil {
			return WooCommercePackage{}, fmt.Errorf("%w: read archive entry: %v", ErrUnsupportedWooCommercePackage, readErr)
		}
		if closeErr != nil {
			return WooCommercePackage{}, fmt.Errorf("%w: close archive entry: %v", ErrUnsupportedWooCommercePackage, closeErr)
		}
		if len(content) > maxWooCommerceEntryBytes || uint64(len(content)) != file.UncompressedSize64 {
			return WooCommercePackage{}, fmt.Errorf("%w: archive entry size is invalid", ErrUnsupportedWooCommercePackage)
		}
		if strings.Join(parts[1:], "/") == WooCommerceSortingModuleName+".php" {
			entryPoint = content
			entryPath = cleaned
		}
	}
	if root != expectedRoot || len(entryPoint) == 0 {
		return WooCommercePackage{}, fmt.Errorf("%w: unexpected package root or missing plugin entry point", ErrUnsupportedWooCommercePackage)
	}
	if !regexp.MustCompile(`(?m)^\s*\*\s*Plugin Name:\s*Extra Product Sorting Options for WooCommerce\s*$`).Match(entryPoint) ||
		!regexp.MustCompile(`(?m)^\s*\*\s*Version:\s*2\.11\.0\s*$`).Match(entryPoint) {
		return WooCommercePackage{}, fmt.Errorf("%w: plugin metadata does not match the pinned release", ErrUnsupportedWooCommercePackage)
	}
	sourceDigest := sha256.Sum256(entryPoint)
	if hex.EncodeToString(sourceDigest[:]) != WooCommerceSortingSourceSHA256 {
		return WooCommercePackage{}, fmt.Errorf("%w: plugin source hash does not match the pinned release", ErrUnsupportedWooCommercePackage)
	}
	return WooCommercePackage{
		Name:       WooCommerceSortingModuleName,
		Version:    WooCommerceSortingModuleVersion,
		EntryPath:  entryPath,
		EntryPoint: entryPoint,
		ArchiveSHA: archiveSHA,
	}, nil
}

func BuildWooCommerceSortingArchive(data []byte) (ModuleIR, []byte, error) {
	pkg, err := ReadWooCommerceSortingArchive(data)
	if err != nil {
		return ModuleIR{}, nil, err
	}
	analysis, err := AnalyzePHPSource(string(pkg.EntryPoint))
	if err != nil {
		return ModuleIR{}, nil, fmt.Errorf("analyze WooCommerce entry point: %w", err)
	}
	provenance, err := NewPackageProvenance(pkg.EntryPoint, data, SourceProvenance{
		SourceReference: WooCommerceSortingSourceReference,
		ModuleVersion:   pkg.Version,
		PlatformVersion: "9.7.1",
		Adapter:         "woocommerce",
		AdapterVersion:  wooCommerceAdapterVersion,
	})
	if err != nil {
		return ModuleIR{}, nil, err
	}
	ir, err := LowerPHPAnalysisToIR(pkg.Name, analysis, provenance)
	if err != nil {
		return ModuleIR{}, nil, fmt.Errorf("lower WooCommerce entry point: %w", err)
	}
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		return ModuleIR{}, nil, fmt.Errorf("compile WooCommerce module: %w", err)
	}
	return ir, wasmBytes, nil
}
