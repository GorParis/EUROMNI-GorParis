package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"

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
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": map[string]string{
		"order_id": result.OrderID, "payment_id": result.PaymentID, "outcome": result.Outcome,
		"payment_status": paymentStatus, "order_status": orderStatus,
	}})
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
