package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"euromni/internal/artifact"
	"euromni/internal/store"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

type publishArtifactStore struct {
	artifact artifact.Artifact
	putErr   error
	putCalls int
	putBytes []byte
}

func (s *publishArtifactStore) Put(_ context.Context, source io.Reader) (artifact.Artifact, error) {
	s.putCalls++
	if s.putErr != nil {
		return artifact.Artifact{}, s.putErr
	}
	var err error
	s.putBytes, err = io.ReadAll(source)
	if err != nil {
		return artifact.Artifact{}, err
	}
	return s.artifact, nil
}

func (*publishArtifactStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("unexpected artifact read")
}

func TestPublishModuleBuildRejectsMissingDependencies(t *testing.T) {
	validIR := testCatalogIR()
	validModules := store.NewSQL(nil)
	validArtifacts := &publishArtifactStore{}

	tests := []struct {
		name      string
		artifacts artifact.Store
		modules   *store.Store
	}{
		{name: "artifact store", modules: validModules},
		{name: "module store", artifacts: validArtifacts},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := PublishModuleBuild(context.Background(), test.artifacts, test.modules, "module-id", validIR, []byte("wasm")); err == nil {
				t.Fatal("PublishModuleBuild() accepted a missing store")
			}
		})
	}
}

func TestPublishModuleBuildValidatesBeforeStoringArtifact(t *testing.T) {
	artifacts := &publishArtifactStore{}
	invalidIR := testCatalogIR()
	invalidIR.Module = ""

	if _, _, err := PublishModuleBuild(context.Background(), artifacts, store.NewSQL(nil), "module-id", invalidIR, []byte("wasm")); err == nil {
		t.Fatal("PublishModuleBuild() accepted invalid IR")
	}
	if artifacts.putCalls != 0 {
		t.Fatalf("artifact Put() called %d times for invalid IR, want 0", artifacts.putCalls)
	}
	if _, _, err := PublishModuleBuild(context.Background(), artifacts, store.NewSQL(nil), "module-id", testCatalogIR(), nil); err == nil {
		t.Fatal("PublishModuleBuild() accepted empty Wasm")
	}
	if artifacts.putCalls != 0 {
		t.Fatalf("artifact Put() called %d times for empty Wasm, want 0", artifacts.putCalls)
	}
}

func TestPublishModuleBuildRejectsIncompatibleWasmBeforeStoringArtifact(t *testing.T) {
	ir := testCatalogIR()
	irJSON, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	wasmBytes, err := wasmtime.Wat2Wasm(`(module
  (import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
  (import "wasi_snapshot_preview1" "proc_exit" (func (param i32)))
  (memory (export "memory") 17)
  (func (export "run") (result i32) (i32.const 0))
)`)
	if err != nil {
		t.Fatal(err)
	}
	irHash := sha256.Sum256(irJSON)
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmIRHashSection, []byte(hex.EncodeToString(irHash[:])))
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmCompilerSection, []byte(IRCompilerVersion))

	artifacts := &publishArtifactStore{}
	if _, _, err := PublishModuleBuild(context.Background(), artifacts, store.NewSQL(nil), "module-id", ir, wasmBytes); err == nil {
		t.Fatal("PublishModuleBuild() accepted Wasm with an additional import")
	}
	if artifacts.putCalls != 0 {
		t.Fatalf("artifact Put() called %d times for incompatible Wasm, want 0", artifacts.putCalls)
	}
}

func TestPublishModuleBuildPropagatesArtifactStoreError(t *testing.T) {
	putErr := errors.New("artifact storage unavailable")
	artifacts := &publishArtifactStore{putErr: putErr}
	wasmBytes, err := CompileIRToWasm(testCatalogIR())
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = PublishModuleBuild(context.Background(), artifacts, store.NewSQL(nil), "module-id", testCatalogIR(), wasmBytes)
	if !errors.Is(err, putErr) {
		t.Fatalf("PublishModuleBuild() error = %v, want wrapped artifact error", err)
	}
	if artifacts.putCalls != 1 {
		t.Fatalf("artifact Put() called %d times, want 1", artifacts.putCalls)
	}
}

func TestPublishModuleBuildRejectsInconsistentArtifactMetadata(t *testing.T) {
	wasmBytes, err := CompileIRToWasm(testCatalogIR())
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wasmBytes)
	artifacts := &publishArtifactStore{
		artifact: artifact.Artifact{SHA256: hex.EncodeToString(digest[:]), Size: int64(len(wasmBytes) + 1)},
	}

	_, _, err = PublishModuleBuild(context.Background(), artifacts, store.NewSQL(nil), "module-id", testCatalogIR(), wasmBytes)
	if err == nil || !strings.Contains(err.Error(), "inconsistent with build manifest") {
		t.Fatalf("PublishModuleBuild() error = %v, want inconsistent metadata error", err)
	}
	if !bytes.Equal(artifacts.putBytes, wasmBytes) {
		t.Fatalf("stored Wasm = %q, want %q", artifacts.putBytes, wasmBytes)
	}
}

func TestPublishModuleBuildRegistersOnlyAfterArtifactValidation(t *testing.T) {
	wasmBytes, err := CompileIRToWasm(testCatalogIR())
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(wasmBytes)
	artifacts := &publishArtifactStore{
		artifact: artifact.Artifact{SHA256: hex.EncodeToString(digest[:]), Size: int64(len(wasmBytes))},
	}

	_, _, err = PublishModuleBuild(context.Background(), artifacts, store.NewSQL(nil), "module-id", testCatalogIR(), wasmBytes)
	if !errors.Is(err, sql.ErrConnDone) {
		t.Fatalf("PublishModuleBuild() error = %v, want module registration to be attempted", err)
	}
	if !bytes.Equal(artifacts.putBytes, wasmBytes) {
		t.Fatalf("stored Wasm = %q, want %q", artifacts.putBytes, wasmBytes)
	}
}
