package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"euromni/internal/artifact"
	"euromni/internal/pipeline"
	"euromni/internal/scheduler"
	"euromni/internal/store"
)

func main() {
	logger := log.New(os.Stdout, "euromni-artifact-reconciler ", log.LstdFlags|log.LUTC)
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		logger.Fatal("DATABASE_URL is required")
	}
	artifactRoot := strings.TrimSpace(os.Getenv("EUROMNI_ARTIFACT_DIR"))
	if artifactRoot == "" {
		logger.Fatal("EUROMNI_ARTIFACT_DIR is required")
	}
	interval, err := durationFromEnv("EUROMNI_ARTIFACT_RECONCILIATION_INTERVAL", pipeline.DefaultArtifactReconciliationInterval)
	if err != nil {
		logger.Fatal(err)
	}
	gracePeriod, err := durationFromEnv("EUROMNI_ARTIFACT_ORPHAN_GRACE_PERIOD", pipeline.DefaultArtifactOrphanGracePeriod)
	if err != nil {
		logger.Fatal(err)
	}
	retryInterval, err := durationFromEnv("EUROMNI_ARTIFACT_RECONCILIATION_RETRY_INTERVAL", pipeline.DefaultArtifactReconciliationRetryInterval)
	if err != nil {
		logger.Fatal(err)
	}

	moduleStore, err := store.Open(databaseURL)
	if err != nil {
		logger.Fatalf("open module registry: %v", err)
	}
	if moduleStore.SQL == nil {
		logger.Fatal("module registry database is unavailable")
	}
	defer moduleStore.SQL.Close()

	artifactStore, err := artifact.NewFileStore(artifactRoot)
	if err != nil {
		logger.Fatalf("open artifact store: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Printf("starting; interval=%s retry_interval=%s orphan_grace=%s artifact_dir=%s", interval, retryInterval, gracePeriod, artifactRoot)
	err = scheduler.RunWithRetry(ctx, interval, retryInterval, func(ctx context.Context) error {
		removed, err := pipeline.ReconcileModuleArtifactsWithGrace(ctx, artifactStore, moduleStore, gracePeriod)
		if err != nil {
			return err
		}
		logger.Printf("reconciliation completed; removed=%d", len(removed))
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatalf("scheduler stopped: %v", err)
	}
	logger.Print("stopped")
}

func durationFromEnv(name string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, errors.New(name + " must be a positive Go duration")
	}
	return duration, nil
}
