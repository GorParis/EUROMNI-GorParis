package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunExecutesImmediatelyAndPeriodically(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	err := RunWithRetry(ctx, time.Millisecond, time.Millisecond, func(context.Context) error {
		if calls.Add(1) == 3 {
			cancel()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("job calls = %d, want immediate run plus two scheduled runs", calls.Load())
	}
}

func TestRunContinuesAfterJobFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	err := RunWithRetry(ctx, time.Hour, time.Millisecond, func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("temporary database failure")
		}
		cancel()
		return nil
	})
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("job calls = %d, want retry after initial failure", calls.Load())
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	if err := RunWithRetry(context.Background(), 0, time.Second, func(context.Context) error { return nil }); err == nil {
		t.Fatal("Run() accepted a zero interval")
	}
	if err := RunWithRetry(context.Background(), time.Second, 0, func(context.Context) error { return nil }); err == nil {
		t.Fatal("Run() accepted a zero retry interval")
	}
	if err := RunWithRetry(context.Background(), time.Second, time.Second, nil); err == nil {
		t.Fatal("Run() accepted a nil job")
	}
}
