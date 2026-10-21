package pipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

func TestBuildManifestBindsSourceIRAndWasm(t *testing.T) {
	ir := testCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("CompileIRToWasm() returned error: %v", err)
	}
	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatalf("NewBuildManifest() returned error: %v", err)
	}
	if err := manifest.Validate(ir, wasmBytes); err != nil {
		t.Fatalf("Validate() rejected matching build inputs: %v", err)
	}
	if manifest.Source.SourceSHA256 != ir.Provenance.SourceSHA256 || manifest.ABIVersion != "euromni.abi.v1" || manifest.WASMSize != int64(len(wasmBytes)) {
		t.Fatalf("manifest = %+v, missing expected provenance or artifact metadata", manifest)
	}

	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var decoded BuildManifest
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if err := decoded.Validate(ir, wasmBytes); err != nil {
		t.Fatalf("Validate() rejected serialized manifest: %v", err)
	}
}

func TestBuildManifestRejectsChangedSourceIROrWasm(t *testing.T) {
	ir := testCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("CompileIRToWasm() returned error: %v", err)
	}

	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatalf("NewBuildManifest() returned error: %v", err)
	}

	modifiedIR := testCatalogIR()
	modifiedIR.Module = "demo.other-module"
	if err := manifest.Validate(modifiedIR, wasmBytes); err == nil {
		t.Fatal("manifest accepted a different IR")
	}
	modifiedWasm := append([]byte(nil), wasmBytes...)
	modifiedWasm[len(modifiedWasm)-1] ^= 1
	if err := manifest.Validate(ir, modifiedWasm); err == nil {
		t.Fatal("manifest accepted different Wasm bytes")
	}
	manifest.Capabilities = []string{"filesystem"}
	if err := manifest.Validate(ir, wasmBytes); err == nil {
		t.Fatal("manifest accepted capabilities not derived from the IR")
	}
}

func TestCompiledWasmBindsExactIRInCustomSection(t *testing.T) {
	firstIR := testCatalogIR()
	first, err := CompileIRToWasm(firstIR)
	if err != nil {
		t.Fatalf("compile first IR: %v", err)
	}
	secondIR := testCatalogIR()
	secondIR.Provenance.PlatformVersion = "9.1"
	second, err := CompileIRToWasm(secondIR)
	if err != nil {
		t.Fatalf("compile second IR: %v", err)
	}
	if string(first) == string(second) {
		t.Fatal("different IR provenance produced identical Wasm bytes")
	}
	firstIRBytes, err := json.Marshal(firstIR)
	if err != nil {
		t.Fatal(err)
	}
	firstIRHash := sha256.Sum256(firstIRBytes)
	if !bytes.Contains(first, []byte(hex.EncodeToString(firstIRHash[:]))) {
		t.Fatal("compiled Wasm is missing the exact IR hash custom section")
	}
	engine := wasmtime.NewEngine()
	defer engine.Close()
	if _, err := wasmtime.NewModule(engine, first); err != nil {
		t.Fatalf("Wasmtime rejected the provenance custom section: %v", err)
	}
}
