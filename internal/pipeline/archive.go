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
	for _, archived := range reader.File {
		if archived.FileInfo().IsDir() {
			continue
		}
		if archived.FileInfo().Mode()&os.ModeSymlink != 0 {
			return ErrPHPArchiveUnsafe
		}
		name := strings.ReplaceAll(archived.Name, "\\", "/")
		if name == "" || name == "." || strings.Contains(name, "\x00") {
			return ErrPHPArchiveUnsafe
		}
		if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || name == ".." || strings.Contains(name, "/../") || strings.Contains(name, "..//") || strings.Contains(name, "..\\") {
			return ErrPHPArchiveUnsafe
		}
		cleaned := path.Clean(name)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return ErrPHPArchiveUnsafe
		}
		if strings.HasPrefix(cleaned, "/") || strings.Contains(cleaned, "//") {
			return ErrPHPArchiveUnsafe
		}

		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".php", ".json", ".md", ".txt":
		default:
			return ErrPHPArchiveUnexpectedFileType
		}
		if strings.EqualFold(name, "manifest.json") || strings.HasSuffix(strings.ToLower(name), "/manifest.json") {
			seenManifest = true
		}
		if ext == ".php" {
			seenPHP = true
		}
		if archived.UncompressedSize64 > MaxEntryBytes {
			return ErrPHPArchiveUnsafe
		}
		if archived.UncompressedSize64 == 0 && archived.Method == zip.Store {
			continue
		}
		opened, err := archived.Open()
		if err != nil {
			return fmt.Errorf("%w: open %q: %v", ErrPHPArchiveInvalid, archived.Name, err)
		}
		_, copyErr := io.CopyN(io.Discard, opened, MaxEntryBytes+1)
		_ = opened.Close()
		if copyErr == nil {
			return ErrPHPArchiveUnsafe
		}
		if !errors.Is(copyErr, io.EOF) && !errors.Is(copyErr, io.ErrUnexpectedEOF) {
			return fmt.Errorf("%w: read %q: %v", ErrPHPArchiveInvalid, archived.Name, copyErr)
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
