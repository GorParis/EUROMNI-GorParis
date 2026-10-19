package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"euromni/internal/store"
)

type SupplierOrderProcessor interface {
	SubmitOrder(ctx context.Context, order store.SupplierOrderRequest, idempotencyKey string) (string, error)
}

var selfnamedOrderProcessor SupplierOrderProcessor

func SetSelfnamedOrderProcessor(processor SupplierOrderProcessor) {
	selfnamedOrderProcessor = processor
}

func syncSelfnamedOrder(ctx context.Context, shopID, orderID string) (*store.SupplierOrderSyncStatus, string, error) {
	if selfnamedOrderProcessor == nil {
		return nil, "disabled", nil
	}
	syncStore, ok := currentProductReader().(store.SupplierOrderSyncStore)
	if !ok {
		return nil, "unavailable", errors.New("supplier order sync store unavailable")
	}
	claim, err := syncStore.ClaimSupplierOrderSync(ctx, shopID, orderID, "selfnamed")
	if errors.Is(err, store.ErrSupplierOrderHasNoItems) {
		return nil, "not_applicable", nil
	}
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return nil, "not_found", err
		case errors.Is(err, store.ErrSupplierOrderNotReady):
			return nil, "not_ready", err
		case errors.Is(err, store.ErrSupplierOrderSyncInProgress):
			return nil, "syncing", err
		}
		return nil, "failed", err
	}
	if claim.AlreadySynced {
		return &claim.Status, "synced", nil
	}
	externalOrderID, err := selfnamedOrderProcessor.SubmitOrder(ctx, claim.Order, claim.IdempotencyKey)
	if err == nil && strings.TrimSpace(externalOrderID) == "" {
		err = errors.New("supplier provider returned an empty order ID")
	}
	if err != nil {
		failErr := syncStore.FailSupplierOrderSync(ctx, shopID, orderID, "selfnamed", claim.IdempotencyKey, err.Error())
		if failErr != nil {
			return nil, "failed", fmt.Errorf("submit supplier order: %v; persist failure: %w", err, failErr)
		}
		status := claim.Status
		status.Status = "failed"
		status.LastError = err.Error()
		return &status, "failed", err
	}
	if err := syncStore.CompleteSupplierOrderSync(ctx, shopID, orderID, "selfnamed", claim.IdempotencyKey, externalOrderID); err != nil {
		return nil, "syncing", fmt.Errorf("complete supplier order sync: %w", err)
	}
	status := claim.Status
	status.Status = "synced"
	status.ExternalOrderID = externalOrderID
	status.LastError = ""
	return &status, "synced", nil
}

func adminOrderSupplierSync(w http.ResponseWriter, r *http.Request) {
	if !orderIDPattern.MatchString(r.PathValue("id")) {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid order id"})
		return
	}
	if selfnamedOrderProcessor == nil {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "Selfnamed mock provider unavailable"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	status, outcome, err := syncSelfnamedOrder(r.Context(), session.ShopID, r.PathValue("id"))
	if err != nil {
		log.Printf("retry Selfnamed order sync %s: %v", r.PathValue("id"), err)
		statusCode := http.StatusBadGateway
		switch {
		case errors.Is(err, store.ErrNotFound):
			statusCode = http.StatusNotFound
		case errors.Is(err, store.ErrSupplierOrderNotReady),
			errors.Is(err, store.ErrSupplierOrderSyncInProgress):
			statusCode = http.StatusConflict
		}
		writePrivateJSON(w, statusCode, map[string]any{
			"error": "supplier order sync failed", "supplier_order_sync_status": outcome,
		})
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"status": outcome, "supplier_order_sync": status},
	})
}
