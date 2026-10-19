package api

import (
	"context"
	"errors"
	"testing"

	"euromni/internal/store"
)

type supplierSyncTestReader struct {
	store.ProductReader
	store.SupplierOrderSyncStore
}

type supplierSyncTestStore struct {
	claim       store.SupplierOrderSyncClaim
	claimErr    error
	completedID string
	failed      string
}

func (s *supplierSyncTestStore) ClaimSupplierOrderSync(context.Context, string, string, string) (*store.SupplierOrderSyncClaim, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	claim := s.claim
	return &claim, nil
}

func TestSyncSelfnamedOrderReportsActiveLease(t *testing.T) {
	syncStore := &supplierSyncTestStore{claimErr: store.ErrSupplierOrderSyncInProgress}
	originalReader, originalProcessor := productReader, selfnamedOrderProcessor
	SetProductReader(supplierSyncTestReader{SupplierOrderSyncStore: syncStore})
	SetSelfnamedOrderProcessor(&supplierSyncTestProcessor{})
	t.Cleanup(func() {
		SetProductReader(originalReader)
		SetSelfnamedOrderProcessor(originalProcessor)
	})

	_, outcome, err := syncSelfnamedOrder(context.Background(), "shop-1", "order-1")
	if !errors.Is(err, store.ErrSupplierOrderSyncInProgress) || outcome != "syncing" {
		t.Fatalf("sync result = %q, error = %v; want active syncing lease", outcome, err)
	}
}

func (s *supplierSyncTestStore) CompleteSupplierOrderSync(_ context.Context, _, _, _, key, externalOrderID string) error {
	if key != s.claim.IdempotencyKey {
		return store.ErrOrderTransition
	}
	s.completedID = externalOrderID
	return nil
}

func (s *supplierSyncTestStore) FailSupplierOrderSync(_ context.Context, _, _, _, _, reason string) error {
	s.failed = reason
	return nil
}

func (s *supplierSyncTestStore) GetSupplierOrderSyncStatus(context.Context, string, string, string) (*store.SupplierOrderSyncStatus, error) {
	return nil, store.ErrNotFound
}

type supplierSyncTestProcessor struct {
	calls int
	err   error
}

func (p *supplierSyncTestProcessor) SubmitOrder(_ context.Context, _ store.SupplierOrderRequest, key string) (string, error) {
	p.calls++
	if p.err != nil {
		return "", p.err
	}
	return "external-" + key, nil
}

func TestSyncSelfnamedOrderCompletesWithStableIdempotencyKey(t *testing.T) {
	syncStore := &supplierSyncTestStore{
		claim: store.SupplierOrderSyncClaim{
			IdempotencyKey: "stable-key",
			Status:         store.SupplierOrderSyncStatus{Status: "syncing", Attempts: 1},
			Order:          store.SupplierOrderRequest{OrderID: "order-1"},
		},
	}
	processor := &supplierSyncTestProcessor{}
	originalReader, originalProcessor := productReader, selfnamedOrderProcessor
	SetProductReader(supplierSyncTestReader{SupplierOrderSyncStore: syncStore})
	SetSelfnamedOrderProcessor(processor)
	t.Cleanup(func() {
		SetProductReader(originalReader)
		SetSelfnamedOrderProcessor(originalProcessor)
	})

	status, outcome, err := syncSelfnamedOrder(context.Background(), "shop-1", "order-1")
	if err != nil {
		t.Fatalf("syncSelfnamedOrder() error = %v", err)
	}
	if outcome != "synced" || status == nil || status.Status != "synced" ||
		status.ExternalOrderID != "external-stable-key" || syncStore.completedID != status.ExternalOrderID ||
		processor.calls != 1 {
		t.Fatalf("sync result/status = %q/%+v, completed ID = %q, calls = %d",
			outcome, status, syncStore.completedID, processor.calls)
	}
}

func TestSyncSelfnamedOrderRecordsProviderFailure(t *testing.T) {
	syncStore := &supplierSyncTestStore{
		claim: store.SupplierOrderSyncClaim{
			IdempotencyKey: "stable-key",
			Status:         store.SupplierOrderSyncStatus{Status: "syncing", Attempts: 1},
			Order:          store.SupplierOrderRequest{OrderID: "order-1"},
		},
	}
	processor := &supplierSyncTestProcessor{err: errors.New("mock rejection")}
	originalReader, originalProcessor := productReader, selfnamedOrderProcessor
	SetProductReader(supplierSyncTestReader{SupplierOrderSyncStore: syncStore})
	SetSelfnamedOrderProcessor(processor)
	t.Cleanup(func() {
		SetProductReader(originalReader)
		SetSelfnamedOrderProcessor(originalProcessor)
	})

	status, outcome, err := syncSelfnamedOrder(context.Background(), "shop-1", "order-1")
	if err == nil || outcome != "failed" || status == nil || status.Status != "failed" ||
		syncStore.failed != "mock rejection" || syncStore.completedID != "" {
		t.Fatalf("sync result/status = %q/%+v, failure = %q, error = %v",
			outcome, status, syncStore.failed, err)
	}
}
