package selfnamed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"euromni/internal/store"
)

type Mock struct{}

func (Mock) SubmitOrder(ctx context.Context, order store.SupplierOrderRequest, idempotencyKey string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if idempotencyKey == "" || order.OrderID == "" || len(order.Items) == 0 {
		return "", errors.New("incomplete Selfnamed mock order")
	}
	for _, item := range order.Items {
		if item.SupplierSKU == "" || item.Quantity <= 0 {
			return "", errors.New("invalid Selfnamed mock order item")
		}
	}
	digest := sha256.Sum256([]byte(idempotencyKey))
	return "selfnamed-mock-" + hex.EncodeToString(digest[:12]), nil
}
