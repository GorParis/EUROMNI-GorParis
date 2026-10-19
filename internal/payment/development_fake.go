package payment

import (
	"context"
	"fmt"

	"euromni/internal/store"
)

type DevelopmentFake struct{}

func (DevelopmentFake) Process(ctx context.Context, attempt store.PaymentAttempt, testCase string) (store.PaymentResult, error) {
	if err := ctx.Err(); err != nil {
		return store.PaymentResult{}, err
	}
	outcome := ""
	switch testCase {
	case "approved":
		outcome = "accepted"
	case "declined":
		outcome = "declined"
	case "abandoned":
		outcome = "abandoned"
	default:
		return store.PaymentResult{}, fmt.Errorf("unsupported development payment test case %q", testCase)
	}
	return store.PaymentResult{
		PaymentID: attempt.ID, OrderID: attempt.OrderID, Provider: attempt.Provider,
		Amount: attempt.Amount, Currency: attempt.Currency, Outcome: outcome,
		EventID:   "fake-payment-" + attempt.ID + "-" + outcome,
		Reference: "fake-reference-" + attempt.ID,
	}, nil
}
