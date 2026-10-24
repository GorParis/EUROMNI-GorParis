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
const MaxDecisionHistoryBytes = 256 << 10
const MaxDecisionHistoryEntries = 256
const MaxDecisionReasonBytes = 1000

var (
	ErrCandidateEmpty               = errors.New("build package candidate is empty")
	ErrCandidateLarge               = errors.New("build package candidate exceeds size limit")
	ErrCandidateStoreFull           = errors.New("shop build package candidate storage limit reached")
	ErrCandidateCorrupt             = errors.New("build package candidate failed integrity check")
	ErrCandidateNotFound            = errors.New("build package candidate not found")
	ErrCandidateApproved            = errors.New("build package candidate already approved")
	ErrCandidateDecisionConflict    = errors.New("build package candidate decision conflicts with current state")
	ErrCandidateReasonRequired      = errors.New("build package candidate rejection reason is required")
	ErrCandidateReasonInvalid       = errors.New("build package candidate rejection reason is invalid")
	ErrCandidateDecisionHistoryFull = errors.New("build package candidate decision history is full")
)

const (
	DecisionApprove = "approve"
	DecisionReject  = "reject"
	DecisionReopen  = "reopen"
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

type Decision struct {
	Action    string `json:"action"`
	ActorID   string `json:"actor_id"`
	ActorRole string `json:"actor_role"`
	At        string `json:"at"`
	Reason    string `json:"reason,omitempty"`
}

type CandidateReview struct {
	Candidate
	Status    string     `json:"status"`
	Approval  *Approval  `json:"approval,omitempty"`
	Rejection *Decision  `json:"rejection,omitempty"`
	History   []Decision `json:"decision_history,omitempty"`
}

type Store interface {
	Put(context.Context, string, []byte) (Candidate, error)
	Get(context.Context, string, string) ([]byte, error)
	List(context.Context, string, int, int) ([]CandidateReview, int, error)
	GetReview(context.Context, string, string) (CandidateReview, error)
	GetApproval(context.Context, string, string) (*Approval, error)
	Approve(context.Context, string, string, string, string) (Approval, error)
	Decide(context.Context, string, string, string, string, string, string) (CandidateReview, error)
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

func (s *FileStore) List(ctx context.Context, shopID string, limit, offset int) ([]CandidateReview, int, error) {
	if s == nil || s.root == "" {
		return nil, 0, errors.New("build package candidate store is not initialized")
	}
	if strings.TrimSpace(shopID) == "" {
		return nil, 0, errors.New("shop ID is required")
	}
	if limit < 1 || offset < 0 {
		return nil, 0, errors.New("candidate list limit and offset are invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	directory := filepath.Join(s.root, shopDigest(shopID))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []CandidateReview{}, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list shop build package candidates: %w", err)
	}

	candidates := make([]CandidateReview, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		digest := entry.Name()
		if !validDigest(digest) {
			continue
		}
		info, err := os.Lstat(filepath.Join(directory, digest))
		if err != nil {
			return nil, 0, fmt.Errorf("inspect build package candidate: %w", err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
			info.Size() <= 0 || info.Size() > pipeline.MaxBuildPackageArchiveBytes {
			return nil, 0, ErrCandidateCorrupt
		}
		review, err := readCandidateReview(directory, digest, info.Size())
		if err != nil {
			return nil, 0, err
		}
		review.History = nil
		candidates = append(candidates, review)
	}

	total := len(candidates)
	if offset >= total {
		return []CandidateReview{}, total, nil
	}
	end := offset + limit
	if end < offset || end > total {
		end = total
	}
	return candidates[offset:end], total, nil
}

func (s *FileStore) GetReview(ctx context.Context, shopID, digest string) (CandidateReview, error) {
	if s == nil || s.root == "" {
		return CandidateReview{}, errors.New("build package candidate store is not initialized")
	}
	if strings.TrimSpace(shopID) == "" || !validDigest(digest) {
		return CandidateReview{}, errors.New("shop ID and valid package digest are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CandidateReview{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	directory := filepath.Join(s.root, shopDigest(shopID))
	data, err := verifyCandidateFile(filepath.Join(directory, digest), digest)
	if errors.Is(err, os.ErrNotExist) {
		return CandidateReview{}, ErrCandidateNotFound
	}
	if err != nil {
		return CandidateReview{}, err
	}
	return readCandidateReview(directory, digest, int64(len(data)))
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
	review, err := s.GetReview(ctx, shopID, digest)
	if err != nil {
		return nil, err
	}
	return review.Approval, nil
}

func (s *FileStore) Approve(ctx context.Context, shopID, digest, actorID, actorRole string) (Approval, error) {
	review, err := s.Decide(ctx, shopID, digest, actorID, actorRole, DecisionApprove, "")
	if err != nil {
		return Approval{}, err
	}
	if review.Approval == nil {
		return Approval{}, ErrCandidateCorrupt
	}
	return *review.Approval, nil
}

func (s *FileStore) Decide(ctx context.Context, shopID, digest, actorID, actorRole, action, reason string) (CandidateReview, error) {
	if s == nil || s.root == "" {
		return CandidateReview{}, errors.New("build package candidate store is not initialized")
	}
	if strings.TrimSpace(shopID) == "" || !validDigest(digest) || strings.TrimSpace(actorID) == "" {
		return CandidateReview{}, errors.New("shop ID, valid package digest, and decision actor are required")
	}
	if actorRole != "owner" && actorRole != "admin" {
		return CandidateReview{}, errors.New("decision actor must have owner or admin role")
	}
	reason = strings.TrimSpace(reason)
	switch action {
	case DecisionApprove, DecisionReopen:
		if reason != "" {
			return CandidateReview{}, errors.New("reason is only valid for a rejection")
		}
	case DecisionReject:
		if reason == "" {
			return CandidateReview{}, ErrCandidateReasonRequired
		}
		if len(reason) > MaxDecisionReasonBytes {
			return CandidateReview{}, ErrCandidateReasonInvalid
		}
	default:
		return CandidateReview{}, errors.New("candidate decision is invalid")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CandidateReview{}, err
	}
	directory := filepath.Join(s.root, shopDigest(shopID))

	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := verifyCandidateFile(filepath.Join(directory, digest), digest)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return CandidateReview{}, ErrCandidateNotFound
		}
		return CandidateReview{}, err
	}
	review, err := readCandidateReview(directory, digest, int64(len(data)))
	if err != nil {
		return CandidateReview{}, err
	}
	if action == DecisionApprove && review.Status == "approved" {
		return CandidateReview{}, ErrCandidateApproved
	}
	if (action == DecisionReject && review.Status != "pending") ||
		(action == DecisionReopen && review.Status != "rejected") ||
		(action == DecisionApprove && review.Status != "pending") {
		return CandidateReview{}, ErrCandidateDecisionConflict
	}
	if len(review.History) >= MaxDecisionHistoryEntries {
		return CandidateReview{}, ErrCandidateDecisionHistoryFull
	}
	decision := Decision{
		Action:    action,
		ActorID:   actorID,
		ActorRole: actorRole,
		At:        time.Now().UTC().Format(time.RFC3339Nano),
	}
	if action == DecisionReject {
		decision.Reason = reason
	}
	review.History = append(review.History, decision)
	switch action {
	case DecisionApprove:
		review.Status = "approved"
		review.Approval = approvalFromDecision(digest, decision)
		review.Rejection = nil
	case DecisionReject:
		review.Status = "rejected"
		review.Approval = nil
		review.Rejection = &decision
	case DecisionReopen:
		review.Status = "pending"
		review.Approval = nil
		review.Rejection = nil
	}
	historyPath := filepath.Join(directory, digest+".decisions.json")
	if err := writeDecisionHistory(historyPath, review.History); err != nil {
		return CandidateReview{}, err
	}
	return review, nil
}

func approvalFromDecision(digest string, decision Decision) *Approval {
	return &Approval{
		PackageSHA256: digest,
		ApprovedBy:    decision.ActorID,
		ApproverRole:  decision.ActorRole,
		ApprovedAt:    decision.At,
	}
}

func readCandidateReview(directory, digest string, size int64) (CandidateReview, error) {
	history, err := readDecisionHistory(filepath.Join(directory, digest+".decisions.json"), filepath.Join(directory, digest+".approval.json"), digest)
	if err != nil {
		return CandidateReview{}, err
	}
	review := CandidateReview{
		Candidate: Candidate{SHA256: digest, Size: size},
		Status:    "pending",
		History:   history,
	}
	for _, decision := range history {
		switch decision.Action {
		case DecisionApprove:
			review.Status = "approved"
			review.Approval = approvalFromDecision(digest, decision)
			review.Rejection = nil
		case DecisionReject:
			review.Status = "rejected"
			review.Approval = nil
			review.Rejection = &decision
		case DecisionReopen:
			review.Status = "pending"
			review.Approval = nil
			review.Rejection = nil
		}
	}
	return review, nil
}

func readDecisionHistory(path, legacyApprovalPath, digest string) ([]Decision, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		legacy, legacyErr := readApproval(legacyApprovalPath, digest)
		if errors.Is(legacyErr, os.ErrNotExist) {
			return []Decision{}, nil
		}
		if legacyErr != nil {
			return nil, legacyErr
		}
		return []Decision{{
			Action: DecisionApprove, ActorID: legacy.ApprovedBy,
			ActorRole: legacy.ApproverRole, At: legacy.ApprovedAt,
		}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect build package decision history: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > MaxDecisionHistoryBytes {
		return nil, ErrCandidateCorrupt
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open build package decision history: %w", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, MaxDecisionHistoryBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read build package decision history: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close build package decision history: %w", closeErr)
	}
	if len(data) > MaxDecisionHistoryBytes {
		return nil, ErrCandidateCorrupt
	}
	var history []Decision
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil {
		return nil, fmt.Errorf("%w: decode decision history: %v", ErrCandidateCorrupt, err)
	}
	if history == nil || len(history) > MaxDecisionHistoryEntries {
		return nil, ErrCandidateCorrupt
	}
	if err := validateDecisionHistory(history); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(history)
	if err != nil || !bytes.Equal(data, canonical) {
		return nil, ErrCandidateCorrupt
	}
	return history, nil
}

func validateDecisionHistory(history []Decision) error {
	state := "pending"
	for _, decision := range history {
		if strings.TrimSpace(decision.ActorID) == "" ||
			(decision.ActorRole != "owner" && decision.ActorRole != "admin") {
			return ErrCandidateCorrupt
		}
		if _, err := time.Parse(time.RFC3339Nano, decision.At); err != nil {
			return ErrCandidateCorrupt
		}
		switch decision.Action {
		case DecisionApprove:
			if state != "pending" || decision.Reason != "" {
				return ErrCandidateCorrupt
			}
			state = "approved"
		case DecisionReject:
			if state != "pending" || strings.TrimSpace(decision.Reason) == "" ||
				decision.Reason != strings.TrimSpace(decision.Reason) ||
				len(decision.Reason) > MaxDecisionReasonBytes {
				return ErrCandidateCorrupt
			}
			state = "rejected"
		case DecisionReopen:
			if state != "rejected" || decision.Reason != "" {
				return ErrCandidateCorrupt
			}
			state = "pending"
		default:
			return ErrCandidateCorrupt
		}
	}
	return nil
}

func writeDecisionHistory(path string, history []Decision) error {
	data, err := json.Marshal(history)
	if err != nil {
		return fmt.Errorf("encode build package decision history: %w", err)
	}
	if len(data) > MaxDecisionHistoryBytes {
		return ErrCandidateDecisionHistoryFull
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".package-decisions-*")
	if err != nil {
		return fmt.Errorf("create build package decision history staging file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure build package decision history: %w", err)
	}
	written, err := temporary.Write(data)
	if err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write build package decision history: %w", err)
	}
	if written != len(data) {
		_ = temporary.Close()
		return errors.New("write build package decision history: short write")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync build package decision history: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close build package decision history: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("record build package decision history: %w", err)
	}
	return nil
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
