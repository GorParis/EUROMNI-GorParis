package pipeline

import (
	"context"
	"errors"
	"time"

	"euromni/internal/artifact"
	"euromni/internal/store"
)

const (
	DefaultArtifactReconciliationInterval      = 24 * time.Hour
	DefaultArtifactReconciliationRetryInterval = 5 * time.Minute
	DefaultArtifactOrphanGracePeriod           = 7 * 24 * time.Hour
)

func ReconcileModuleArtifacts(ctx context.Context, artifacts *artifact.FileStore, modules *store.Store) ([]artifact.Artifact, error) {
	return ReconcileModuleArtifactsWithGrace(ctx, artifacts, modules, DefaultArtifactOrphanGracePeriod)
}

func ReconcileModuleArtifactsWithGrace(ctx context.Context, artifacts *artifact.FileStore, modules *store.Store, gracePeriod time.Duration) ([]artifact.Artifact, error) {
	if artifacts == nil || modules == nil {
		return nil, errors.New("artifact and module stores are required")
	}
	referenced, err := modules.ListModuleWASMDigests(ctx)
	if err != nil {
		return nil, err
	}
	return artifacts.PruneUnreferenced(ctx, referenced, gracePeriod)
}
