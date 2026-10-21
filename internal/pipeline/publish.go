package pipeline

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"euromni/internal/artifact"
	"euromni/internal/store"
)

func PublishModuleBuild(ctx context.Context, artifacts artifact.Store, modules *store.Store, moduleID string, ir ModuleIR, wasmBytes []byte) (artifact.Artifact, BuildManifest, error) {
	if artifacts == nil || modules == nil {
		return artifact.Artifact{}, BuildManifest{}, errors.New("artifact and module stores are required")
	}
	storedArtifact, manifest, digest, manifestJSON, err := storeBuildArtifact(ctx, artifacts, ir, wasmBytes)
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, err
	}
	if err := modules.RegisterModuleBuild(ctx, moduleID, digest, manifestJSON); err != nil {
		return artifact.Artifact{}, BuildManifest{}, fmt.Errorf("register module build: %w", err)
	}
	return storedArtifact, manifest, nil
}

func PublishModuleBuildOnly(ctx context.Context, artifacts artifact.Store, modules *store.Store, moduleID string, ir ModuleIR, wasmBytes []byte) (artifact.Artifact, BuildManifest, error) {
	if artifacts == nil || modules == nil {
		return artifact.Artifact{}, BuildManifest{}, errors.New("artifact and module stores are required")
	}
	storedArtifact, manifest, digest, manifestJSON, err := storeBuildArtifact(ctx, artifacts, ir, wasmBytes)
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, err
	}
	if err := modules.RegisterModuleBuildOnly(ctx, moduleID, digest, manifestJSON); err != nil {
		return artifact.Artifact{}, BuildManifest{}, fmt.Errorf("register module build without promotion: %w", err)
	}
	return storedArtifact, manifest, nil
}

func PublishNewProductsModuleBuild(ctx context.Context, artifacts artifact.Store, modules *store.Store, shopID string, settings store.NewProductsSettings, ir ModuleIR, wasmBytes []byte) (artifact.Artifact, BuildManifest, string, error) {
	if artifacts == nil || modules == nil {
		return artifact.Artifact{}, BuildManifest{}, "", errors.New("artifact and module stores are required")
	}
	storedArtifact, manifest, digest, manifestJSON, err := storeBuildArtifact(ctx, artifacts, ir, wasmBytes)
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, "", err
	}
	moduleID, err := modules.InstallNewProductsModuleBuild(ctx, shopID, digest, manifestJSON, settings)
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, "", fmt.Errorf("install PrestaShop module build: %w", err)
	}
	return storedArtifact, manifest, moduleID, nil
}

func storeBuildArtifact(ctx context.Context, artifacts artifact.Store, ir ModuleIR, wasmBytes []byte) (artifact.Artifact, BuildManifest, []byte, []byte, error) {
	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, nil, nil, err
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, nil, nil, fmt.Errorf("encode build manifest: %w", err)
	}
	storedArtifact, err := artifacts.Put(ctx, bytes.NewReader(wasmBytes))
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, nil, nil, fmt.Errorf("store Wasm artifact: %w", err)
	}
	if storedArtifact.SHA256 != manifest.WASMSHA256 || storedArtifact.Size != manifest.WASMSize {
		return artifact.Artifact{}, BuildManifest{}, nil, nil, errors.New("artifact store returned metadata inconsistent with build manifest")
	}
	digest, err := hex.DecodeString(storedArtifact.SHA256)
	if err != nil {
		return artifact.Artifact{}, BuildManifest{}, nil, nil, errors.New("artifact store returned an invalid SHA-256")
	}
	return storedArtifact, manifest, digest, manifestJSON, nil
}
