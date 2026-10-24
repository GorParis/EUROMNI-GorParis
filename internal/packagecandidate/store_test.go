package packagecandidate

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileStorePutScopesCandidatesByShopAndIsIdempotent(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "candidates"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	data := []byte("signed-package")
	first, err := store.Put(ctx, "shop-one", data)
	if err != nil {
		t.Fatalf("Put() returned error: %v", err)
	}
	second, err := store.Put(ctx, "shop-one", data)
	if err != nil {
		t.Fatalf("idempotent Put() returned error: %v", err)
	}
	if first != second || first.Size != int64(len(data)) {
		t.Fatalf("candidate receipts = %+v and %+v", first, second)
	}
	loaded, err := store.Get(ctx, "shop-one", first.SHA256)
	if err != nil || !bytes.Equal(loaded, data) {
		t.Fatalf("Get() = %q, error %v", loaded, err)
	}
	if _, err := store.Get(ctx, "shop-two", first.SHA256); err == nil {
		t.Fatal("Get() returned another shop's candidate")
	}
}

func TestFileStoreRejectsInvalidInputsAndCorruptCandidates(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "candidates"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "shop", nil); !errors.Is(err, ErrCandidateEmpty) {
		t.Fatalf("Put(empty) error = %v, want %v", err, ErrCandidateEmpty)
	}
	if _, err := store.Get(context.Background(), "shop", "invalid"); err == nil {
		t.Fatal("Get() accepted an invalid digest")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Put(ctx, "shop", []byte("candidate")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put(cancelled) error = %v, want context cancellation", err)
	}
}

func TestFileStoreRecordsImmutableShopScopedApproval(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "candidates"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	candidate, err := store.Put(ctx, "shop-one", []byte("signed-package"))
	if err != nil {
		t.Fatal(err)
	}
	if approval, err := store.GetApproval(ctx, "shop-one", candidate.SHA256); err != nil || approval != nil {
		t.Fatalf("pending approval = %+v, error %v", approval, err)
	}
	approval, err := store.Approve(ctx, "shop-one", candidate.SHA256, "staff-123", "owner")
	if err != nil {
		t.Fatalf("Approve() returned error: %v", err)
	}
	if approval.PackageSHA256 != candidate.SHA256 || approval.ApprovedBy != "staff-123" ||
		approval.ApproverRole != "owner" || approval.ApprovedAt == "" {
		t.Fatalf("approval metadata = %+v", approval)
	}
	recorded, err := store.GetApproval(ctx, "shop-one", candidate.SHA256)
	if err != nil || recorded == nil || *recorded != approval {
		t.Fatalf("GetApproval() = %+v, error %v", recorded, err)
	}
	if _, err := store.GetApproval(ctx, "shop-two", candidate.SHA256); !errors.Is(err, ErrCandidateNotFound) {
		t.Fatalf("cross-shop GetApproval() error = %v, want %v", err, ErrCandidateNotFound)
	}
	if _, err := store.Approve(ctx, "shop-one", candidate.SHA256, "staff-456", "admin"); !errors.Is(err, ErrCandidateApproved) {
		t.Fatalf("second Approve() error = %v, want %v", err, ErrCandidateApproved)
	}
	if _, err := store.Approve(ctx, "shop-one", candidate.SHA256, "staff-456", "editor"); err == nil {
		t.Fatal("Approve() accepted an editor")
	}
}

func TestFileStoreListsCandidatesWithStablePaginationAndStatus(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "candidates"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	receipts := make([]Candidate, 0, 3)
	for _, data := range [][]byte{[]byte("package-one"), []byte("package-two"), []byte("package-three")} {
		candidate, err := store.Put(ctx, "shop-one", data)
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, candidate)
	}
	approvedCandidate := receipts[1]
	if _, err := store.Approve(ctx, "shop-one", approvedCandidate.SHA256, "staff-123", "admin"); err != nil {
		t.Fatal(err)
	}

	firstPage, total, err := store.List(ctx, "shop-one", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(firstPage) != 2 || firstPage[0].SHA256 >= firstPage[1].SHA256 {
		t.Fatalf("first page / total = %+v / %d", firstPage, total)
	}
	secondPage, secondTotal, err := store.List(ctx, "shop-one", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if secondTotal != total || len(secondPage) != 1 {
		t.Fatalf("second page / total = %+v / %d", secondPage, secondTotal)
	}
	foundApproval := false
	for _, item := range append(firstPage, secondPage...) {
		if item.SHA256 == approvedCandidate.SHA256 {
			foundApproval = item.Status == "approved" && item.Approval != nil && item.Approval.ApprovedBy == "staff-123"
		}
	}
	if !foundApproval {
		t.Fatal("approved candidate was not represented with its approval metadata")
	}
	emptyPage, emptyTotal, err := store.List(ctx, "shop-two", 50, 0)
	if err != nil || len(emptyPage) != 0 || emptyTotal != 0 {
		t.Fatalf("other shop list / total / error = %+v / %d / %v", emptyPage, emptyTotal, err)
	}
}

func TestFileStoreRejectReopenAndApprovePreservesDecisionHistory(t *testing.T) {
	store, err := NewFileStore(filepath.Join(t.TempDir(), "candidates"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	candidate, err := store.Put(ctx, "shop-one", []byte("signed-package"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-1", "admin", DecisionReject, ""); !errors.Is(err, ErrCandidateReasonRequired) {
		t.Fatalf("Reject(empty reason) error = %v, want %v", err, ErrCandidateReasonRequired)
	}
	if _, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-1", "admin", DecisionReject, strings.Repeat("x", MaxDecisionReasonBytes+1)); !errors.Is(err, ErrCandidateReasonInvalid) {
		t.Fatalf("Reject(oversized reason) error = %v, want %v", err, ErrCandidateReasonInvalid)
	}
	rejected, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-1", "admin", DecisionReject, "Provenance requires clarification")
	if err != nil {
		t.Fatalf("Reject() returned error: %v", err)
	}
	if rejected.Status != "rejected" || rejected.Rejection == nil ||
		rejected.Rejection.ActorID != "staff-1" ||
		rejected.Rejection.Reason != "Provenance requires clarification" ||
		len(rejected.History) != 1 {
		t.Fatalf("rejected review = %+v", rejected)
	}
	if _, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-2", "owner", DecisionReject, "second rejection"); !errors.Is(err, ErrCandidateDecisionConflict) {
		t.Fatalf("second rejection error = %v, want %v", err, ErrCandidateDecisionConflict)
	}
	if _, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-2", "owner", DecisionApprove, ""); !errors.Is(err, ErrCandidateDecisionConflict) {
		t.Fatalf("approval while rejected error = %v, want %v", err, ErrCandidateDecisionConflict)
	}
	reopened, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-2", "owner", DecisionReopen, "")
	if err != nil {
		t.Fatalf("Reopen() returned error: %v", err)
	}
	if reopened.Status != "pending" || len(reopened.History) != 2 || reopened.History[1].Action != DecisionReopen {
		t.Fatalf("reopened review = %+v", reopened)
	}
	if _, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-2", "owner", DecisionReopen, ""); !errors.Is(err, ErrCandidateDecisionConflict) {
		t.Fatalf("reopen while pending error = %v, want %v", err, ErrCandidateDecisionConflict)
	}
	approved, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-3", "admin", DecisionApprove, "")
	if err != nil {
		t.Fatalf("Approve after reopening returned error: %v", err)
	}
	if approved.Status != "approved" || approved.Approval == nil || len(approved.History) != 3 {
		t.Fatalf("approved review = %+v", approved)
	}
	recorded, err := store.GetReview(ctx, "shop-one", candidate.SHA256)
	if err != nil || recorded.Status != "approved" || len(recorded.History) != 3 ||
		recorded.History[0].Reason != "Provenance requires clarification" ||
		recorded.History[2].ActorID != "staff-3" {
		t.Fatalf("recorded review = %+v, error %v", recorded, err)
	}
	if _, err := store.Decide(ctx, "shop-one", candidate.SHA256, "staff-4", "owner", DecisionReject, "withdraw approval"); !errors.Is(err, ErrCandidateDecisionConflict) {
		t.Fatalf("rejection after approval error = %v, want %v", err, ErrCandidateDecisionConflict)
	}
}
