package scheduler

import (
	"context"
	"errors"
	"log"
	"time"
)

func Run(ctx context.Context, interval time.Duration, job func(context.Context) error) error {
	return RunWithRetry(ctx, interval, interval, job)
}

func RunWithRetry(ctx context.Context, interval, retryInterval time.Duration, job func(context.Context) error) error {
	if interval <= 0 {
		return errors.New("scheduler interval must be positive")
	}
	if retryInterval <= 0 {
		return errors.New("scheduler retry interval must be positive")
	}
	if job == nil {
		return errors.New("scheduler job is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return nil
	}

	runJob := func() bool {
		if err := job(ctx); err != nil && ctx.Err() == nil {
			log.Printf("scheduled task failed: %v", err)
			return false
		}
		return true
	}
	succeeded := runJob()
	for {
		wait := interval
		if !succeeded {
			wait = retryInterval
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
			succeeded = runJob()
		}
	}
}
