package pipeline

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	MaxPHPArchiveBytes = 16 << 20
	MaxPHPArchiveFiles = 128
	MaxEntryBytes      = 2 << 20
)

var (
	ErrPHPArchiveEmpty              = errors.New("php archive is empty")
	ErrPHPArchiveTooLarge           = errors.New("php archive exceeds size limit")
	ErrPHPArchiveInvalid            = errors.New("php archive is malformed")
	ErrPHPArchiveUnsafe             = errors.New("php archive contains unsafe paths or entries")
	ErrPHPArchiveUnexpectedFileType = errors.New("php archive contains unexpected files")
	ErrPHPArchiveMissingManifest    = errors.New("php archive is missing manifest.json")
	ErrPHPManifestInvalid           = errors.New("php module manifest is invalid")
)

type PHPManifest struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Shop     string `json:"shop"`
	Source   struct {
		Type string `json:"type"`
	} `json:"source,omitempty"`
}

func ParsePHPManifest(data []byte) (*PHPManifest, error) {
	if err := ValidatePHPArchive(data); err != nil {
		return nil, err
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPHPArchiveInvalid, err)
	}

	for _, archived := range reader.File {
		if archived.Name == "manifest.json" || strings.HasSuffix(strings.ToLower(archived.Name), "/manifest.json") {
			r, err := archived.Open()
			if err != nil {
				return nil, fmt.Errorf("%w: open manifest: %v", ErrPHPManifestInvalid, err)
			}
			manifestBytes, err := io.ReadAll(io.LimitReader(r, MaxEntryBytes+1))
			_ = r.Close()
			if err != nil {
				return nil, fmt.Errorf("%w: read manifest: %v", ErrPHPManifestInvalid, err)
			}
			if len(manifestBytes) > MaxEntryBytes {
				return nil, ErrPHPArchiveUnsafe
			}

			var manifest PHPManifest
			if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
				return nil, fmt.Errorf("%w: invalid JSON: %v", ErrPHPManifestInvalid, err)
			}
			if strings.TrimSpace(manifest.Name) == "" || strings.TrimSpace(manifest.Version) == "" || strings.TrimSpace(manifest.Platform) == "" {
				return nil, fmt.Errorf("%w: name, version and platform are required", ErrPHPManifestInvalid)
			}
			if strings.TrimSpace(manifest.Shop) == "" {
				return nil, fmt.Errorf("%w: shop is required", ErrPHPManifestInvalid)
			}
			if manifest.Platform != "woocommerce" && manifest.Platform != "prestashop" {
				return nil, fmt.Errorf("%w: unsupported platform %q", ErrPHPManifestInvalid, manifest.Platform)
			}
			return &manifest, nil
		}
	}
	return nil, ErrPHPArchiveMissingManifest
}

func ValidatePHPArchive(data []byte) error {
	if len(data) == 0 {
		return ErrPHPArchiveEmpty
	}
	if len(data) > MaxPHPArchiveBytes {
		return ErrPHPArchiveTooLarge
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPHPArchiveInvalid, err)
	}
	if len(reader.File) == 0 {
		return ErrPHPArchiveInvalid
	}
	if len(reader.File) > MaxPHPArchiveFiles {
		return ErrPHPArchiveUnsafe
	}

	seenManifest := false
	seenPHP := false
	seenPaths := make(map[string]struct{}, len(reader.File))
	var totalUncompressed uint64
	for _, archived := range reader.File {
		name := strings.ReplaceAll(archived.Name, "\\", "/")
		directory := archived.FileInfo().IsDir()
		cleanName := strings.TrimSuffix(name, "/")
		cleaned := path.Clean(cleanName)
		if name == "" || cleanName == "" || strings.ContainsRune(name, '\x00') ||
			cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") ||
			strings.HasPrefix(cleaned, "/") || cleaned != cleanName || strings.Contains(cleaned, ":") {
			return ErrPHPArchiveUnsafe
		}
		mode := archived.Mode()
		if mode&os.ModeSymlink != 0 || (mode.Type() != 0 && mode.Type() != os.ModeDir) ||
			(directory && mode.Type() != 0 && mode.Type() != os.ModeDir) ||
			(!directory && mode.IsDir()) {
			return ErrPHPArchiveUnsafe
		}
		pathKey := strings.ToLower(cleaned)
		if _, duplicate := seenPaths[pathKey]; duplicate {
			return ErrPHPArchiveUnsafe
		}
		seenPaths[pathKey] = struct{}{}
		if directory {
			if archived.UncompressedSize64 != 0 || archived.CompressedSize64 != 0 {
				return ErrPHPArchiveUnsafe
			}
			continue
		}

		ext := strings.ToLower(filepath.Ext(cleaned))
		switch ext {
		case ".php", ".json", ".md", ".txt":
		default:
			return ErrPHPArchiveUnexpectedFileType
		}
		if strings.EqualFold(cleaned, "manifest.json") || strings.HasSuffix(strings.ToLower(cleaned), "/manifest.json") {
			seenManifest = true
		}
		if ext == ".php" {
			seenPHP = true
		}
		if archived.UncompressedSize64 > MaxEntryBytes ||
			totalUncompressed > (32<<20)-archived.UncompressedSize64 {
			return ErrPHPArchiveUnsafe
		}
		totalUncompressed += archived.UncompressedSize64
		opened, err := archived.Open()
		if err != nil {
			return fmt.Errorf("%w: open %q: %v", ErrPHPArchiveInvalid, archived.Name, err)
		}
		entrySize, readErr := io.Copy(io.Discard, io.LimitReader(opened, MaxEntryBytes+1))
		closeErr := opened.Close()
		if readErr != nil {
			return fmt.Errorf("%w: read %q: %v", ErrPHPArchiveInvalid, archived.Name, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("%w: close %q: %v", ErrPHPArchiveInvalid, archived.Name, closeErr)
		}
		if entrySize > MaxEntryBytes || uint64(entrySize) != archived.UncompressedSize64 {
			return ErrPHPArchiveUnsafe
		}
	}
	if !seenManifest {
		return ErrPHPArchiveMissingManifest
	}
	if !seenPHP {
		return ErrPHPArchiveUnexpectedFileType
	}
	return nil
}
