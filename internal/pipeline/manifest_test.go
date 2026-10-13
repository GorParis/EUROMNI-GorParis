package pipeline

import (
	"encoding/json"
	"testing"
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
