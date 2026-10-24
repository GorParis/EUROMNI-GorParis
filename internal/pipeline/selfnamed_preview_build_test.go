//go:build wasmtoolchain

package pipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"euromni/internal/buildmeta"
)

func TestCompileSelfnamedPreviewIRToWasmIsReproducibleAndBoundToIR(t *testing.T) {
	ir := testSelfnamedPreviewIR()
	first, err := CompileSelfnamedPreviewIRToWasm(ir)
	if err != nil {
		t.Fatalf("compile first Selfnamed preview build: %v", err)
	}
	second, err := CompileSelfnamedPreviewIRToWasm(ir)
	if err != nil {
		t.Fatalf("compile second Selfnamed preview build: %v", err)
	}
	if !bytes.Equal(first, second) {
		firstHash := sha256.Sum256(first)
		secondHash := sha256.Sum256(second)
		t.Fatalf("identical Selfnamed preview IR produced different Wasm bytes: %s != %s",
			hex.EncodeToString(firstHash[:]), hex.EncodeToString(secondHash[:]))
	}
	irBytes, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	irHash := sha256.Sum256(irBytes)
	if !wasmHasCustomSection(first, wasmIRHashSection, []byte(hex.EncodeToString(irHash[:]))) {
		t.Fatal("compiled guest does not contain the exact IR hash")
	}
	if !wasmHasCustomSection(first, wasmCompilerSection, []byte(buildmeta.SelfnamedPreviewCompiler)) {
		t.Fatal("compiled guest does not contain the pinned compiler identity")
	}
	manifest, err := NewBuildManifest(ir, first)
	if err != nil {
		t.Fatalf("create build manifest: %v", err)
	}
	if manifest.Compiler != buildmeta.SelfnamedPreviewCompiler {
		t.Fatalf("manifest compiler = %q, want %q", manifest.Compiler, buildmeta.SelfnamedPreviewCompiler)
	}
	if err := manifest.Validate(ir, first); err != nil {
		t.Fatalf("validate matching build manifest: %v", err)
	}
	modifiedIR := testSelfnamedPreviewIR()
	modifiedIR.Provenance.PlatformVersion = "other-platform-version"
	if err := manifest.Validate(modifiedIR, first); err == nil {
		t.Fatal("build manifest accepted Wasm compiled from a different IR")
	}
}
