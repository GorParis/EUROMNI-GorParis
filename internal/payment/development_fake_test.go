package payment

import (
	"context"
	"testing"

	"euromni/internal/store"
)

func TestDevelopmentFakeProducesDeterministicResults(t *testing.T) {
	provider := DevelopmentFake{}
	attempt := store.PaymentAttempt{
		ID: "payment-1", OrderID: "order-1", Provider: "development-fake",
		Amount: "42.0000", Currency: "EUR", Status: "pending",
	}
	for _, test := range []struct {
		testCase string
		outcome  string
	}{
		{testCase: "approved", outcome: "accepted"},
		{testCase: "declined", outcome: "declined"},
		{testCase: "abandoned", outcome: "abandoned"},
	} {
		t.Run(test.testCase, func(t *testing.T) {
			first, err := provider.Process(context.Background(), attempt, test.testCase)
			if err != nil {
				t.Fatalf("process test case: %v", err)
			}
			second, err := provider.Process(context.Background(), attempt, test.testCase)
			if err != nil {
				t.Fatalf("repeat test case: %v", err)
			}
			if first != second || first.Outcome != test.outcome ||
				first.OrderID != attempt.OrderID || first.PaymentID != attempt.ID ||
				first.Amount != attempt.Amount || first.Currency != attempt.Currency {
				t.Fatalf("repeated provider results = %+v / %+v, want deterministic %s result", first, second, test.outcome)
			}
		})
	}
	if _, err := provider.Process(context.Background(), attempt, "unknown"); err == nil {
		t.Fatal("unsupported test case was accepted")
	}
}
