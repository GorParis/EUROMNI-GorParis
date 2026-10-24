package packagecandidate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"euromni/internal/pipeline"
	"euromni/internal/privatefs"
)

const MaxShopCandidateBytes = 256 << 20

var (
	ErrCandidateEmpty     = errors.New("build package candidate is empty")
	ErrCandidateLarge     = errors.New("build package candidate exceeds size limit")
	ErrCandidateStoreFull = errors.New("shop build package candidate storage limit reached")
	ErrCandidateCorrupt   = errors.New("build package candidate failed integrity check")
	ErrCandidateNotFound  = errors.New("build package candidate not found")
	ErrCandidateApproved  = errors.New("build package candidate already approved")
)

type Candidate struct {
	SHA256 string `json:"package_sha256"`
	Size   int64  `json:"size"`
}

type Approval struct {
	PackageSHA256 string `json:"package_sha256"`
	ApprovedBy    string `json:"approved_by"`
	ApproverRole  string `json:"approver_role"`
	ApprovedAt    string `json:"approved_at"`
}

type Store interface {
	Put(context.Context, string, []byte) (Candidate, error)
	Get(context.Context, string, string) ([]byte, error)
	GetApproval(context.Context, string, string) (*Approval, error)
	Approve(context.Context, string, string, string, string) (Approval, error)
}

type FileStore struct {
	root string
	mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("build package candidate root is required")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve build package candidate root: %w", err)
	}
	if err := privatefs.EnsureDirectory(absoluteRoot); err != nil {
		return nil, fmt.Errorf("configure private build package candidate directory: %w", err)
	}
	return &FileStore{root: absoluteRoot}, nil
}

func (s *FileStore) Put(ctx context.Context, shopID string, data []byte) (Candidate, error) {
	if s == nil || s.root == "" {
		return Candidate{}, errors.New("build package candidate store is not initialized")
	}
	if strings.TrimSpace(shopID) == "" {
		return Candidate{}, errors.New("shop ID is required")
	}
	if len(data) == 0 {
		return Candidate{}, ErrCandidateEmpty
	}
	if len(data) > pipeline.MaxBuildPackageArchiveBytes {
		return Candidate{}, ErrCandidateLarge
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Candidate{}, err
	}

	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	directory := filepath.Join(s.root, shopDigest(shopID))

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := privatefs.EnsureDirectory(directory); err != nil {
		return Candidate{}, fmt.Errorf("create shop build package candidate directory: %w", err)
	}
	target := filepath.Join(directory, digest)
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return Candidate{}, ErrCandidateCorrupt
		}
		existing, err := readCandidate(target, digest)
		if err != nil {
			return Candidate{}, err
		}
		return Candidate{SHA256: digest, Size: int64(len(existing))}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Candidate{}, fmt.Errorf("inspect build package candidate: %w", err)
	}

	used, err := candidateDirectorySize(directory)
	if err != nil {
		return Candidate{}, err
	}
	if used > MaxShopCandidateBytes-int64(len(data)) {
		return Candidate{}, ErrCandidateStoreFull
	}
	if err := ctx.Err(); err != nil {
		return Candidate{}, err
	}

	temporary, err := os.CreateTemp(directory, ".package-candidate-*")
	if err != nil {
		return Candidate{}, fmt.Errorf("create build package candidate staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return Candidate{}, fmt.Errorf("secure build package candidate staging file: %w", err)
	}
	written, err := temporary.Write(data)
	if err != nil {
		_ = temporary.Close()
		return Candidate{}, fmt.Errorf("write build package candidate: %w", err)
	}
	if written != len(data) {
		_ = temporary.Close()
		return Candidate{}, errors.New("write build package candidate: short write")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Candidate{}, fmt.Errorf("sync build package candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Candidate{}, fmt.Errorf("close build package candidate: %w", err)
	}
	if err := os.Link(temporaryPath, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return Candidate{}, fmt.Errorf("publish build package candidate: %w", err)
		}
		existing, readErr := readCandidate(target, digest)
		if readErr != nil {
			return Candidate{}, readErr
		}
		return Candidate{SHA256: digest, Size: int64(len(existing))}, nil
	}
	return Candidate{SHA256: digest, Size: int64(len(data))}, nil
}

func (s *FileStore) Get(ctx context.Context, shopID, digest string) ([]byte, error) {
	if s == nil || s.root == "" {
		return nil, errors.New("build package candidate store is not initialized")
	}
	if strings.TrimSpace(shopID) == "" || !validDigest(digest) {
		return nil, errors.New("shop ID and valid package digest are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := filepath.Join(s.root, shopDigest(shopID), digest)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrCandidateNotFound
		}
		return nil, fmt.Errorf("inspect build package candidate: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrCandidateCorrupt
	}
	return readCandidate(path, digest)
}

func (s *FileStore) GetApproval(ctx context.Context, shopID, digest string) (*Approval, error) {
	if s == nil || s.root == "" {
		return nil, errors.New("build package candidate store is not initialized")
	}
	if strings.TrimSpace(shopID) == "" || !validDigest(digest) {
		return nil, errors.New("shop ID and valid package digest are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory := filepath.Join(s.root, shopDigest(shopID))
	if _, err := verifyCandidateFile(filepath.Join(directory, digest), digest); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrCandidateNotFound
		}
		return nil, err
	}
	approval, err := readApproval(filepath.Join(directory, digest+".approval.json"), digest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &approval, nil
}

func (s *FileStore) Approve(ctx context.Context, shopID, digest, actorID, actorRole string) (Approval, error) {
	if s == nil || s.root == "" {
		return Approval{}, errors.New("build package candidate store is not initialized")
	}
	if strings.TrimSpace(shopID) == "" || !validDigest(digest) || strings.TrimSpace(actorID) == "" {
		return Approval{}, errors.New("shop ID, valid package digest, and approver ID are required")
	}
	if actorRole != "owner" && actorRole != "admin" {
		return Approval{}, errors.New("approver must have owner or admin role")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Approval{}, err
	}
	directory := filepath.Join(s.root, shopDigest(shopID))

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := verifyCandidateFile(filepath.Join(directory, digest), digest); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Approval{}, ErrCandidateNotFound
		}
		return Approval{}, err
	}
	target := filepath.Join(directory, digest+".approval.json")
	if _, err := os.Lstat(target); err == nil {
		return Approval{}, ErrCandidateApproved
	} else if !errors.Is(err, os.ErrNotExist) {
		return Approval{}, fmt.Errorf("inspect build package approval: %w", err)
	}

	approval := Approval{
		PackageSHA256: digest,
		ApprovedBy:    actorID,
		ApproverRole:  actorRole,
		ApprovedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(approval)
	if err != nil {
		return Approval{}, fmt.Errorf("encode build package approval: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".package-approval-*")
	if err != nil {
		return Approval{}, fmt.Errorf("create build package approval staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return Approval{}, fmt.Errorf("secure build package approval staging file: %w", err)
	}
	written, err := temporary.Write(data)
	if err != nil {
		_ = temporary.Close()
		return Approval{}, fmt.Errorf("write build package approval: %w", err)
	}
	if written != len(data) {
		_ = temporary.Close()
		return Approval{}, errors.New("write build package approval: short write")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return Approval{}, fmt.Errorf("sync build package approval: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Approval{}, fmt.Errorf("close build package approval: %w", err)
	}
	if err := os.Link(temporaryPath, target); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Approval{}, ErrCandidateApproved
		}
		return Approval{}, fmt.Errorf("record build package approval: %w", err)
	}
	return approval, nil
}

func readApproval(path, expectedDigest string) (Approval, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Approval{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 4096 {
		return Approval{}, ErrCandidateCorrupt
	}
	file, err := os.Open(path)
	if err != nil {
		return Approval{}, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, 4097))
	closeErr := file.Close()
	if readErr != nil {
		return Approval{}, fmt.Errorf("%w: read approval: %v", ErrCandidateCorrupt, readErr)
	}
	if closeErr != nil {
		return Approval{}, fmt.Errorf("%w: close approval: %v", ErrCandidateCorrupt, closeErr)
	}
	if len(data) > 4096 {
		return Approval{}, ErrCandidateCorrupt
	}
	var approval Approval
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&approval); err != nil {
		return Approval{}, fmt.Errorf("%w: decode approval: %v", ErrCandidateCorrupt, err)
	}
	if approval.PackageSHA256 != expectedDigest ||
		strings.TrimSpace(approval.ApprovedBy) == "" ||
		(approval.ApproverRole != "owner" && approval.ApproverRole != "admin") {
		return Approval{}, ErrCandidateCorrupt
	}
	if _, err := time.Parse(time.RFC3339Nano, approval.ApprovedAt); err != nil {
		return Approval{}, ErrCandidateCorrupt
	}
	canonical, err := json.Marshal(approval)
	if err != nil || !bytes.Equal(data, canonical) {
		return Approval{}, ErrCandidateCorrupt
	}
	return approval, nil
}

func shopDigest(shopID string) string {
	digest := sha256.Sum256([]byte(shopID))
	return hex.EncodeToString(digest[:])
}

func validDigest(digest string) bool {
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == digest
}

func candidateDirectorySize(directory string) (int64, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, fmt.Errorf("list shop build package candidates: %w", err)
	}
	var total int64
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(directory, entry.Name()))
		if err != nil {
			return 0, fmt.Errorf("inspect shop build package candidate: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return 0, ErrCandidateCorrupt
		}
		if info.Size() < 0 || total > MaxShopCandidateBytes-info.Size() {
			return 0, ErrCandidateStoreFull
		}
		total += info.Size()
	}
	return total, nil
}

func verifyCandidateFile(path, digest string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrCandidateCorrupt
	}
	return readCandidate(path, digest)
}

func readCandidate(path, digest string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open build package candidate: %w", err)
	}

	data, readErr := io.ReadAll(io.LimitReader(file, pipeline.MaxBuildPackageArchiveBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read build package candidate: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close build package candidate: %w", closeErr)
	}
	actual := sha256.Sum256(data)
	if len(data) > pipeline.MaxBuildPackageArchiveBytes || hex.EncodeToString(actual[:]) != digest {
		return nil, ErrCandidateCorrupt
	}
	return data, nil
}
