package pipeline

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"

	"euromni/internal/buildmeta"
)

const (
	MaxPrestaShopArchiveBytes          = 16 << 20
	maxPrestaShopArchiveFiles          = 128
	maxPrestaShopEntryBytes            = 2 << 20
	maxPrestaShopTotalBytes            = 32 << 20
	prestaShopAdapterVersion           = buildmeta.PrestaShopAdapterVersion
	prestaShopNewProductsArchiveSHA256 = "0f0750d8c2a1e6e12fc67169224f597f166b5c80b99cb306f53e9b339b17af77"
)

var ErrUnsupportedPrestaShopPackage = errors.New("unsupported PrestaShop package")

type PrestaShopPackage struct {
	Name       string
	Version    string
	EntryPath  string
	EntryPoint []byte
	ArchiveSHA string
}

func ReadPrestaShopNewProductsArchive(data []byte) (PrestaShopPackage, error) {
	if len(data) == 0 || len(data) > MaxPrestaShopArchiveBytes {
		return PrestaShopPackage{}, fmt.Errorf("%w: archive size is invalid", ErrUnsupportedPrestaShopPackage)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > maxPrestaShopArchiveFiles {
		return PrestaShopPackage{}, fmt.Errorf("%w: invalid ZIP archive", ErrUnsupportedPrestaShopPackage)
	}

	seen := make(map[string]struct{}, len(reader.File))
	root := ""
	var entryPoint, configXML []byte
	var entryPath string
	var totalSize uint64
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if name == "" || strings.ContainsRune(name, '\x00') || strings.HasPrefix(name, "/") {
			return PrestaShopPackage{}, fmt.Errorf("%w: unsafe archive path", ErrUnsupportedPrestaShopPackage)
		}
		cleanName := strings.TrimSuffix(name, "/")
		cleaned := path.Clean(cleanName)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") ||
			strings.HasPrefix(cleaned, "/") || cleaned != cleanName || strings.Contains(cleaned, ":") {
			return PrestaShopPackage{}, fmt.Errorf("%w: unsafe archive path %q", ErrUnsupportedPrestaShopPackage, file.Name)
		}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 || mode.Type() != 0 && mode.Type() != os.ModeDir {
			return PrestaShopPackage{}, fmt.Errorf("%w: symbolic links are not allowed", ErrUnsupportedPrestaShopPackage)
		}
		duplicateKey := strings.ToLower(cleaned)
		if _, ok := seen[duplicateKey]; ok {
			return PrestaShopPackage{}, fmt.Errorf("%w: duplicate archive path %q", ErrUnsupportedPrestaShopPackage, cleaned)
		}
		seen[duplicateKey] = struct{}{}
		parts := strings.Split(cleaned, "/")
		if len(parts) == 0 {
			return PrestaShopPackage{}, fmt.Errorf("%w: package must have one root directory", ErrUnsupportedPrestaShopPackage)
		}
		if root == "" {
			root = parts[0]
		} else if root != parts[0] {
			return PrestaShopPackage{}, fmt.Errorf("%w: package has multiple root directories", ErrUnsupportedPrestaShopPackage)
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if len(parts) < 2 {
			return PrestaShopPackage{}, fmt.Errorf("%w: package must have one root directory", ErrUnsupportedPrestaShopPackage)
		}
		if file.UncompressedSize64 > maxPrestaShopEntryBytes ||
			totalSize > maxPrestaShopTotalBytes-file.UncompressedSize64 {
			return PrestaShopPackage{}, fmt.Errorf("%w: archive expands beyond size limits", ErrUnsupportedPrestaShopPackage)
		}
		totalSize += file.UncompressedSize64
		opened, err := file.Open()
		if err != nil {
			return PrestaShopPackage{}, fmt.Errorf("%w: open archive entry: %v", ErrUnsupportedPrestaShopPackage, err)
		}
		entryData, readErr := io.ReadAll(io.LimitReader(opened, maxPrestaShopEntryBytes+1))
		closeErr := opened.Close()
		if readErr != nil {
			return PrestaShopPackage{}, fmt.Errorf("%w: read archive entry: %v", ErrUnsupportedPrestaShopPackage, readErr)
		}
		if closeErr != nil {
			return PrestaShopPackage{}, fmt.Errorf("%w: close archive entry: %v", ErrUnsupportedPrestaShopPackage, closeErr)
		}
		if len(entryData) > maxPrestaShopEntryBytes || uint64(len(entryData)) != file.UncompressedSize64 {
			return PrestaShopPackage{}, fmt.Errorf("%w: archive entry size is invalid", ErrUnsupportedPrestaShopPackage)
		}
		switch strings.Join(parts[1:], "/") {
		case "ps_newproducts.php":
			entryPoint = entryData
			entryPath = cleaned
		case "config.xml":
			configXML = entryData
		}
	}
	if len(entryPoint) == 0 || len(configXML) == 0 {
		return PrestaShopPackage{}, fmt.Errorf("%w: entry point or config.xml is missing", ErrUnsupportedPrestaShopPackage)
	}
	if root != "ps_newproducts" && !strings.HasPrefix(root, "ps_newproducts-") {
		return PrestaShopPackage{}, fmt.Errorf("%w: unexpected package root directory", ErrUnsupportedPrestaShopPackage)
	}
	moduleName, moduleVersion, err := parsePrestaShopConfig(configXML)
	if err != nil || moduleName != "ps_newproducts" || !validPrestaShopVersion(moduleVersion) {
		return PrestaShopPackage{}, fmt.Errorf("%w: invalid module metadata", ErrUnsupportedPrestaShopPackage)
	}
	if !regexp.MustCompile(`(?m)\$this->name\s*=\s*['"]ps_newproducts['"]\s*;`).Match(entryPoint) {
		return PrestaShopPackage{}, fmt.Errorf("%w: entry point module name does not match config.xml", ErrUnsupportedPrestaShopPackage)
	}
	versionMatch := regexp.MustCompile(`(?m)\$this->version\s*=\s*['"]([^'"]+)['"]\s*;`).FindSubmatch(entryPoint)
	if len(versionMatch) != 2 || string(versionMatch[1]) != moduleVersion {
		return PrestaShopPackage{}, fmt.Errorf("%w: entry point version does not match config.xml", ErrUnsupportedPrestaShopPackage)
	}
	archiveDigest := sha256.Sum256(data)
	return PrestaShopPackage{
		Name:       moduleName,
		Version:    moduleVersion,
		EntryPath:  entryPath,
		EntryPoint: entryPoint,
		ArchiveSHA: hex.EncodeToString(archiveDigest[:]),
	}, nil
}

func BuildPrestaShopNewProductsArchive(data []byte, platformVersion string) (ModuleIR, []byte, error) {
	if len(data) == 0 || len(data) > MaxPrestaShopArchiveBytes {
		return ModuleIR{}, nil, fmt.Errorf("%w: archive size is invalid", ErrUnsupportedPrestaShopPackage)
	}
	archiveDigest := sha256.Sum256(data)
	if hex.EncodeToString(archiveDigest[:]) != prestaShopNewProductsArchiveSHA256 {
		return ModuleIR{}, nil, fmt.Errorf("%w: archive hash is not the pinned source package", ErrUnsupportedPrestaShopPackage)
	}
	pkg, err := ReadPrestaShopNewProductsArchive(data)
	if err != nil {
		return ModuleIR{}, nil, err
	}
	analysis, err := AnalyzePHPSource(string(pkg.EntryPoint))
	if err != nil {
		return ModuleIR{}, nil, fmt.Errorf("analyze PrestaShop entry point: %w", err)
	}
	provenance, err := NewPackageProvenance(pkg.EntryPoint, data, SourceProvenance{
		SourceReference: "sha256:" + pkg.ArchiveSHA,
		ModuleVersion:   pkg.Version,
		PlatformVersion: platformVersion,
		Adapter:         "prestashop",
		AdapterVersion:  prestaShopAdapterVersion,
	})
	if err != nil {
		return ModuleIR{}, nil, err
	}
	ir, err := LowerPHPAnalysisToIR(pkg.Name, analysis, provenance)
	if err != nil {
		return ModuleIR{}, nil, fmt.Errorf("lower PrestaShop entry point: %w", err)
	}
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		return ModuleIR{}, nil, fmt.Errorf("compile PrestaShop module: %w", err)
	}
	return ir, wasmBytes, nil
}

func parsePrestaShopConfig(data []byte) (string, string, error) {
	var config struct {
		XMLName xml.Name `xml:"module"`
		Name    string   `xml:"name"`
		Version string   `xml:"version"`
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", err
		}
		if _, ok := token.(xml.Directive); ok {
			return "", "", errors.New("XML directives are not allowed")
		}
	}
	if err := xml.Unmarshal(data, &config); err != nil {
		return "", "", err
	}
	if config.XMLName.Local != "module" {
		return "", "", errors.New("unexpected module config root")
	}
	return strings.TrimSpace(config.Name), strings.TrimSpace(config.Version), nil
}

func validPrestaShopVersion(version string) bool {
	if version == "" || len(version) > 64 {
		return false
	}
	for _, character := range version {
		if (character < '0' || character > '9') && character != '.' && character != '-' && character != '+' {
			return false
		}
	}
	return true
}
