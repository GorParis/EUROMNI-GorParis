package quarantine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"euromni/internal/privatefs"
)

const (
	MaxArchiveBytes        = 16 << 20
	MaxShopQuarantineBytes = 256 << 20
)

var (
	ErrArchiveEmpty   = errors.New("quarantined archive is empty")
	ErrArchiveLarge   = errors.New("quarantined archive exceeds size limit")
	ErrQuarantineFull = errors.New("shop quarantine storage limit reached")
	ErrArchiveCorrupt = errors.New("quarantined archive failed integrity check")
)

type Archive struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Store interface {
	Put(context.Context, string, []byte) (Archive, error)
}

type FileStore struct {
	root string
	mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("quarantine root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve quarantine root: %w", err)
	}
	if err := privatefs.EnsureDirectory(absoluteRoot); err != nil {
		return nil, fmt.Errorf("configure private quarantine directory: %w", err)
	}
	return &FileStore{root: absoluteRoot}, nil
}

func (s *FileStore) Put(ctx context.Context, scope string, data []byte) (Archive, error) {
	if s == nil || s.root == "" {
		return Archive{}, errors.New("quarantine store is not initialized")
	}
	if strings.TrimSpace(scope) == "" {
		return Archive{}, errors.New("quarantine scope is required")
	}
	if len(data) == 0 {
		return Archive{}, ErrArchiveEmpty
	}
	if len(data) > MaxArchiveBytes {
		return Archive{}, ErrArchiveLarge
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Archive{}, err
	}

	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	directory := filepath.Join(s.root, scopeDigest(scope))

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := privatefs.EnsureDirectory(directory); err != nil {
		return Archive{}, fmt.Errorf("create shop quarantine: %w", err)
	}
	target := filepath.Join(directory, digest)
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return Archive{}, ErrArchiveCorrupt
		}
		existing, err := readArchive(target, digest)
		if err != nil {
			return Archive{}, err
		}
		return Archive{SHA256: digest, Size: int64(len(existing))}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Archive{}, fmt.Errorf("inspect quarantined archive: %w", err)
	}

	used, err := quarantineSize(directory)
	if err != nil {
		return Archive{}, err
	}
	if used > MaxShopQuarantineBytes-int64(len(data)) {
		return Archive{}, ErrQuarantineFull
	}
	if err := ctx.Err(); err != nil {
		return Archive{}, err
	}

	temporary, err := os.CreateTemp(directory, ".quarantine-*")
	if err != nil {
		return Archive{}, fmt.Errorf("create quarantine staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return Archive{}, fmt.Errorf("secure quarantine staging file: %w", err)
	}
	written, err := temporary.Write(data)
	if err != nil {
		_ = temporary.Close()
		return Archive{}, fmt.Errorf("write quarantined archive: %w", err)
	}
	if written != len(data) {
		_ = temporary.Close()
		return Archive{}, errors.New("write quarantined archive: short write")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Archive{}, fmt.Errorf("sync quarantined archive: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Archive{}, fmt.Errorf("close quarantined archive: %w", err)
	}
	if err := os.Link(temporaryPath, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return Archive{}, fmt.Errorf("publish quarantined archive: %w", err)
		}
		existing, readErr := readArchive(target, digest)
		if readErr != nil {
			return Archive{}, readErr
		}
		return Archive{SHA256: digest, Size: int64(len(existing))}, nil
	}
	return Archive{SHA256: digest, Size: int64(len(data))}, nil
}

func (s *FileStore) Get(ctx context.Context, scope, digest string) ([]byte, error) {
	if s == nil || s.root == "" {
		return nil, errors.New("quarantine store is not initialized")
	}
	if strings.TrimSpace(scope) == "" {
		return nil, errors.New("quarantine scope is required")
	}
	if !validDigest(digest) {
		return nil, errors.New("invalid quarantine digest")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory := filepath.Join(s.root, scopeDigest(scope))
	if err := privatefs.EnsureDirectory(directory); err != nil {
		return nil, fmt.Errorf("inspect shop quarantine: %w", err)
	}
	archivePath := filepath.Join(directory, digest)
	info, err := os.Lstat(archivePath)
	if err != nil {
		return nil, fmt.Errorf("inspect quarantined archive: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrArchiveCorrupt
	}
	archive, err := readArchive(archivePath, digest)
	if err != nil {
		return nil, err
	}
	return archive, nil
}

func (s *FileStore) PurgeBefore(ctx context.Context, cutoff time.Time) (int, error) {
	if s == nil || s.root == "" {
		return 0, errors.New("quarantine store is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	scopes, err := os.ReadDir(s.root)
	if err != nil {
		return 0, fmt.Errorf("list quarantine scopes: %w", err)
	}
	var expired []string
	for _, scope := range scopes {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		scopePath := filepath.Join(s.root, scope.Name())
		scopeInfo, err := os.Lstat(scopePath)
		if err != nil {
			return 0, fmt.Errorf("inspect quarantine scope: %w", err)
		}
		if !scopeInfo.IsDir() || !validDigest(scope.Name()) {
			return 0, ErrArchiveCorrupt
		}
		archives, err := os.ReadDir(scopePath)
		if err != nil {
			return 0, fmt.Errorf("list shop quarantine: %w", err)
		}
		for _, archive := range archives {
			archivePath := filepath.Join(scopePath, archive.Name())
			info, err := os.Lstat(archivePath)
			if err != nil {
				return 0, fmt.Errorf("inspect shop quarantine entry: %w", err)
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return 0, ErrArchiveCorrupt
			}
			temporary := strings.HasPrefix(archive.Name(), ".quarantine-")
			if !temporary && !validDigest(archive.Name()) {
				return 0, ErrArchiveCorrupt
			}
			if info.ModTime().Before(cutoff) {
				expired = append(expired, archivePath)
			}
		}
	}

	removed := 0
	for _, path := range expired {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if err := os.Remove(path); err != nil {
			return removed, fmt.Errorf("remove expired quarantined archive: %w", err)
		}
		removed++
	}
	return removed, nil
}

func scopeDigest(scope string) string {
	digest := sha256.Sum256([]byte(scope))
	return hex.EncodeToString(digest[:])
}

func validDigest(digest string) bool {
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == digest
}

func quarantineSize(directory string) (int64, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, fmt.Errorf("list shop quarantine: %w", err)
	}
	var total int64
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(directory, entry.Name()))
		if err != nil {
			return 0, fmt.Errorf("inspect shop quarantine entry: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return 0, ErrArchiveCorrupt
		}
		if !validDigest(entry.Name()) && !strings.HasPrefix(entry.Name(), ".quarantine-") {
			return 0, ErrArchiveCorrupt
		}
		if info.Size() < 0 || total > MaxShopQuarantineBytes-info.Size() {
			return 0, ErrQuarantineFull
		}
		total += info.Size()
	}
	return total, nil
}

func readArchive(path, digest string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open quarantined archive: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, MaxArchiveBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read quarantined archive: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close quarantined archive: %w", closeErr)
	}
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != digest || len(data) > MaxArchiveBytes {
		return nil, ErrArchiveCorrupt
	}
	return data, nil
}
