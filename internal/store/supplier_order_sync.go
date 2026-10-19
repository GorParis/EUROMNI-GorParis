package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const supplierOrderSyncLease = 2 * time.Minute

func SupplierOrderIdempotencyKey(shopID, orderID, supplierCode string) string {
	digest := sha256.Sum256([]byte(shopID + "\x00" + orderID + "\x00" + supplierCode))
	return "euromni-" + hex.EncodeToString(digest[:])
}

func matchingSupplierItems(lines []OrderLineView, supplierCode string) []SupplierOrderItem {
	items := make([]SupplierOrderItem, 0)
	for _, line := range lines {
		for _, supplier := range line.SupplierSnapshot {
			if supplier.Code == supplierCode {
				items = append(items, SupplierOrderItem{
					SupplierSKU: supplier.SKU,
					Name:        line.Name,
					Quantity:    line.Quantity,
				})
				break
			}
		}
	}
	return items
}

type memorySupplierOrderSync struct {
	status         SupplierOrderSyncStatus
	idempotencyKey string
	lockedUntil    time.Time
}

var _ SupplierOrderSyncStore = (*Memory)(nil)

func (m *Memory) ClaimSupplierOrderSync(ctx context.Context, shopID, orderID, supplierCode string) (*SupplierOrderSyncClaim, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if supplierCode == "" {
		return nil, errors.New("supplier code is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return nil, ErrNotFound
	}
	if order.view.Status != "paid" {
		return nil, ErrSupplierOrderNotReady
	}
	items := matchingSupplierItems(order.view.Items, supplierCode)
	if len(items) == 0 {
		return nil, ErrSupplierOrderHasNoItems
	}

	key := orderID + "/" + supplierCode
	syncState, exists := m.supplierSyncs[key]
	if exists && syncState.status.Status == "synced" {
		return &SupplierOrderSyncClaim{
			IdempotencyKey: syncState.idempotencyKey,
			Status:         syncState.status,
			AlreadySynced:  true,
		}, nil
	}
	now := time.Now()
	if exists && syncState.status.Status == "syncing" && now.Before(syncState.lockedUntil) {
		return nil, ErrSupplierOrderSyncInProgress
	}
	idempotencyKey := SupplierOrderIdempotencyKey(shopID, orderID, supplierCode)
	if !exists {
		syncState = &memorySupplierOrderSync{
			idempotencyKey: idempotencyKey,
			status: SupplierOrderSyncStatus{
				OrderID: orderID, SupplierCode: supplierCode,
			},
		}
		m.supplierSyncs[key] = syncState
	}
	syncState.status.Status = "syncing"
	syncState.status.Attempts++
	syncState.status.LastError = ""
	syncState.lockedUntil = now.Add(supplierOrderSyncLease)
	address := order.address
	request := SupplierOrderRequest{
		OrderID: orderID, OrderNumber: order.view.Number,
		BuyerName: order.name, BuyerEmail: order.email,
		Shipping: address, ShippingCode: order.view.ShippingCarrierCode,
		ShippingGroup: order.view.ShippingServiceCode,
		Currency:      order.view.Currency, Items: items,
	}
	return &SupplierOrderSyncClaim{
		IdempotencyKey: idempotencyKey,
		Status:         syncState.status,
		Order:          request,
	}, nil
}

func (m *Memory) CompleteSupplierOrderSync(ctx context.Context, shopID, orderID, supplierCode, idempotencyKey, externalOrderID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	externalOrderID = strings.TrimSpace(externalOrderID)
	if externalOrderID == "" {
		return errors.New("external order id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return ErrNotFound
	}
	syncState, ok := m.supplierSyncs[orderID+"/"+supplierCode]
	if !ok || syncState.idempotencyKey != idempotencyKey || syncState.status.Status != "syncing" {
		return ErrOrderTransition
	}
	syncState.status.Status = "synced"
	syncState.status.ExternalOrderID = externalOrderID
	syncState.status.LastError = ""
	syncState.lockedUntil = time.Time{}
	return nil
}

func (m *Memory) FailSupplierOrderSync(ctx context.Context, shopID, orderID, supplierCode, idempotencyKey, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return ErrNotFound
	}
	syncState, ok := m.supplierSyncs[orderID+"/"+supplierCode]
	if !ok || syncState.idempotencyKey != idempotencyKey || syncState.status.Status != "syncing" {
		return ErrOrderTransition
	}
	syncState.status.Status = "failed"
	syncState.status.LastError = boundedSupplierSyncError(reason)
	syncState.lockedUntil = time.Time{}
	return nil
}

func (m *Memory) GetSupplierOrderSyncStatus(ctx context.Context, shopID, orderID, supplierCode string) (*SupplierOrderSyncStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return nil, ErrNotFound
	}
	syncState, ok := m.supplierSyncs[orderID+"/"+supplierCode]
	if !ok {
		return nil, ErrNotFound
	}
	status := syncState.status
	return &status, nil
}

func boundedSupplierSyncError(reason string) string {
	const maxLength = 512
	if len(reason) > maxLength {
		reason = strings.ToValidUTF8(reason[:maxLength], "")
	}
	return reason
}
