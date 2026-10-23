package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"euromni/internal/seed"
	"euromni/internal/store"
)

type DevelopmentPaymentProcessor interface {
	Process(ctx context.Context, attempt store.PaymentAttempt, testCase string) (store.PaymentResult, error)
}

var developmentPaymentProcessor DevelopmentPaymentProcessor
var orderIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func SetDevelopmentPaymentProcessor(processor DevelopmentPaymentProcessor) {
	developmentPaymentProcessor = processor
}

func developmentPayment(w http.ResponseWriter, r *http.Request) {
	if developmentPaymentProcessor == nil {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "development payment provider unavailable"})
		return
	}
	orderID := r.PathValue("id")
	if !orderIDPattern.MatchString(orderID) {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid order id"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCartRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input struct {
		TestCase string `json:"test_case"`
	}
	if err := decoder.Decode(&input); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body too large"})
			return
		}
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid development payment test case"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid development payment test case"})
		return
	}
	if input.TestCase != "approved" && input.TestCase != "declined" && input.TestCase != "abandoned" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported development payment test case"})
		return
	}
	writer, ok := currentProductReader().(store.PaymentLifecycleWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "payment service unavailable"})
		return
	}
	attempt, err := writer.CreateDevelopmentPaymentAttempt(r.Context(), seed.ShopID, orderID)
	if err != nil {
		writeDevelopmentPaymentError(w, err)
		return
	}
	result, err := developmentPaymentProcessor.Process(r.Context(), *attempt, input.TestCase)
	if err != nil {
		writePrivateJSON(w, http.StatusBadGateway, map[string]any{"error": "development payment provider failed"})
		return
	}
	if err := writer.ApplyDevelopmentPaymentResult(r.Context(), seed.ShopID, result); err != nil {
		if errors.Is(err, store.ErrInvalidPaymentResult) {
			writePrivateJSON(w, http.StatusBadGateway, map[string]any{"error": "payment provider result rejected"})
		} else {
			writeDevelopmentPaymentError(w, err)
		}
		return
	}
	paymentStatus, orderStatus, _ := store.DevelopmentPaymentStatuses(result.Outcome)
	supplierStatus := "not_triggered"
	if orderStatus == "paid" {
		_, supplierStatus, err = syncSelfnamedOrder(r.Context(), seed.ShopID, result.OrderID)
		if err != nil {
			log.Printf("sync Selfnamed order %s after payment: %v", result.OrderID, err)
		}
	}
	data := map[string]any{
		"order_id": result.OrderID, "payment_id": result.PaymentID, "outcome": result.Outcome,
		"payment_status": paymentStatus, "order_status": orderStatus,
		"supplier_order_sync_status": supplierStatus,
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": data})
}

func adminOrderLifecycleDemo(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(os.Getenv("EUROMNI_ENVIRONMENT")) != "development" {
		writePrivateJSON(w, http.StatusNotFound, map[string]any{"error": "local order demo is unavailable"})
		return
	}
	orderID := r.PathValue("id")
	if !orderIDPattern.MatchString(orderID) {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid order id"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	reader, ok := currentProductReader().(store.OrderAdminReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "order administration unavailable"})
		return
	}
	order, err := reader.GetAdminOrder(r.Context(), session.ShopID, orderID)
	if err != nil {
		writeOrderAdminError(w, err)
		return
	}
	if order.Status == "pending" {
		if developmentPaymentProcessor == nil {
			writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "development payment provider unavailable"})
			return
		}
		paymentWriter, ok := currentProductReader().(store.PaymentLifecycleWriter)
		if !ok {
			writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "payment service unavailable"})
			return
		}
		attempt, err := paymentWriter.CreateDevelopmentPaymentAttempt(r.Context(), session.ShopID, orderID)
		if err != nil {
			writeDevelopmentPaymentError(w, err)
			return
		}
		result, err := developmentPaymentProcessor.Process(r.Context(), *attempt, "approved")
		if err != nil {
			writePrivateJSON(w, http.StatusBadGateway, map[string]any{"error": "development payment provider failed"})
			return
		}
		if err := paymentWriter.ApplyDevelopmentPaymentResult(r.Context(), session.ShopID, result); err != nil {
			writeDevelopmentPaymentError(w, err)
			return
		}
		if _, _, err := syncSelfnamedOrder(r.Context(), session.ShopID, orderID); err != nil {
			log.Printf("sync local demo supplier order %s: %v", orderID, err)
		}
	} else if order.Status != "paid" && order.Status != "fulfilling" && order.Status != "fulfilled" {
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "order cannot enter the local demo from its current state"})
		return
	}
	tracking := "DEMO-" + order.Number
	for _, eventType := range []string{"processing", "prepared", "shipped", "delivered"} {
		if err := reader.AdvanceAdminOrderLifecycle(
			r.Context(), session.ShopID, orderID, eventType, tracking, "local_demo",
		); err != nil {
			writeOrderAdminError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeDevelopmentPaymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writePrivateJSON(w, http.StatusNotFound, map[string]any{"error": "order not found"})
	case errors.Is(err, store.ErrOrderTransition):
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "order cannot start or complete this payment"})
	case errors.Is(err, store.ErrInvalidPaymentResult):
		writePrivateJSON(w, http.StatusBadGateway, map[string]any{"error": "payment provider result rejected"})
	default:
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}
