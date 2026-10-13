package artifact

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
)

const MaxArtifactBytes = 64 << 20

var (
	ErrInvalidDigest   = errors.New("invalid artifact digest")
	ErrArtifactTooBig  = errors.New("artifact exceeds size limit")
	ErrArtifactEmpty   = errors.New("artifact is empty")
	ErrArtifactCorrupt = errors.New("artifact failed integrity check")
)

type Artifact struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Store interface {
	Put(ctx context.Context, source io.Reader) (Artifact, error)
	Get(ctx context.Context, digest string) ([]byte, error)
}

type FileStore struct {
	root string
	mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("artifact root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve artifact root: %w", err)
	}
	if err := os.MkdirAll(absoluteRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create artifact root: %w", err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect artifact root: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("artifact root must be a private directory")
	}
	return &FileStore{root: absoluteRoot}, nil
}

func (s *FileStore) Put(ctx context.Context, source io.Reader) (Artifact, error) {
	if s == nil {
		return Artifact{}, errors.New("artifact store is not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.put(ctx, source, MaxArtifactBytes)
}

func (s *FileStore) PruneUnreferenced(ctx context.Context, referenced []string, gracePeriod time.Duration) ([]Artifact, error) {
	if s == nil || s.root == "" {
		return nil, errors.New("artifact store is not initialized")
	}
	if gracePeriod <= 0 {
		return nil, errors.New("artifact prune grace period must be positive")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	keep := make(map[string]struct{}, len(referenced))
	for _, digest := range referenced {
		if !validDigest(digest) {
			return nil, ErrInvalidDigest
		}
		keep[digest] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-gracePeriod)
	shards, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list artifact shards: %w", err)
	}
	removed := make([]Artifact, 0)
	for _, shard := range shards {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if !shard.IsDir() || len(shard.Name()) != 2 || !validHex(shard.Name()) {
			continue
		}
		shardPath := filepath.Join(s.root, shard.Name())
		shardInfo, err := os.Lstat(shardPath)
		if err != nil {
			return removed, fmt.Errorf("inspect artifact shard: %w", err)
		}
		if !shardInfo.IsDir() || shardInfo.Mode()&os.ModeSymlink != 0 {
			continue
		}
		files, err := os.ReadDir(shardPath)
		if err != nil {
			return removed, fmt.Errorf("list artifact shard: %w", err)
		}
		for _, entry := range files {
			if err := ctx.Err(); err != nil {
				return removed, err
			}
			digest := entry.Name()
			if !validDigest(digest) || digest[:2] != shard.Name() {
				continue
			}
			if _, exists := keep[digest]; exists {
				continue
			}
			path := filepath.Join(shardPath, digest)
			info, err := os.Lstat(path)
			if err != nil {
				return removed, fmt.Errorf("inspect artifact: %w", err)
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.Remove(path); err != nil {
				return removed, fmt.Errorf("remove unreferenced artifact: %w", err)
			}
			removed = append(removed, Artifact{SHA256: digest, Size: info.Size()})
		}
	}
	return removed, nil
}

func (s *FileStore) put(ctx context.Context, source io.Reader, maxBytes int64) (Artifact, error) {
	if s == nil || s.root == "" {
		return Artifact{}, errors.New("artifact store is not initialized")
	}
	if source == nil {
		return Artifact{}, ErrArtifactEmpty
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Artifact{}, err
	}
	temporary, err := os.CreateTemp(s.root, ".artifact-*")
	if err != nil {
		return Artifact{}, fmt.Errorf("create temporary artifact: %w", err)
	}
	temporaryName := temporary.Name()
	defer func() { _ = os.Remove(temporaryName) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return Artifact{}, fmt.Errorf("secure temporary artifact: %w", err)
	}
	digestHash := sha256.New()
	limited := io.LimitReader(contextReader{ctx: ctx, reader: source}, maxBytes+1)
	size, err := io.Copy(io.MultiWriter(temporary, digestHash), limited)
	if err != nil {
		_ = temporary.Close()
		return Artifact{}, fmt.Errorf("stream artifact: %w", err)
	}
	if size == 0 {
		_ = temporary.Close()
		return Artifact{}, ErrArtifactEmpty
	}
	if size > maxBytes {
		_ = temporary.Close()
		return Artifact{}, ErrArtifactTooBig
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Artifact{}, fmt.Errorf("sync temporary artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Artifact{}, fmt.Errorf("close temporary artifact: %w", err)
	}
	digest := hex.EncodeToString(digestHash.Sum(nil))
	directory := filepath.Join(s.root, digest[:2])
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create artifact shard: %w", err)
	}
	destination := filepath.Join(directory, digest)
	if err := os.Link(temporaryName, destination); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return Artifact{}, fmt.Errorf("publish artifact: %w", err)
		}
		existing, getErr := s.verifyFile(ctx, destination, digest)
		if getErr != nil {
			return Artifact{}, getErr
		}
		if existing.Size != size {
			return Artifact{}, ErrArtifactCorrupt
		}
	}
	now := time.Now()
	if err := os.Chtimes(destination, now, now); err != nil {
		return Artifact{}, fmt.Errorf("refresh artifact retention time: %w", err)
	}
	return Artifact{SHA256: digest, Size: size}, nil
}

func (s *FileStore) Get(ctx context.Context, digest string) ([]byte, error) {
	if s == nil || s.root == "" {
		return nil, errors.New("artifact store is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validDigest(digest) {
		return nil, ErrInvalidDigest
	}
	path := filepath.Join(s.root, digest[:2], digest)
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > MaxArtifactBytes {
		return nil, ErrArtifactCorrupt
	}
	data, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, reader: file}, MaxArtifactBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	actualHash := sha256.Sum256(data)
	if len(data) != int(info.Size()) || hex.EncodeToString(actualHash[:]) != digest {
		return nil, ErrArtifactCorrupt
	}
	return data, nil
}

func validDigest(digest string) bool {
	if len(digest) != sha256.Size*2 || !validHex(digest) {
		return false
	}
	digestBytes, err := hex.DecodeString(digest)
	return err == nil && hex.EncodeToString(digestBytes) == digest
}

func validHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func (s *FileStore) verifyFile(ctx context.Context, path, digest string) (Artifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return Artifact{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Artifact{}, fmt.Errorf("inspect artifact: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > MaxArtifactBytes {
		return Artifact{}, ErrArtifactCorrupt
	}
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(contextReader{ctx: ctx, reader: file}, MaxArtifactBytes+1))
	if err != nil {
		return Artifact{}, fmt.Errorf("verify artifact: %w", err)
	}
	if size != info.Size() || hex.EncodeToString(hash.Sum(nil)) != digest {
		return Artifact{}, ErrArtifactCorrupt
	}
	return Artifact{SHA256: digest, Size: size}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}
