package pipeline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"euromni/internal/buildmeta"

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

func TestValidateBuildPackageChecksManifestIRAndArtifact(t *testing.T) {
	ir := testCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	irJSON, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBuildPackage(manifestJSON, irJSON, wasmBytes); err != nil {
		t.Fatalf("ValidateBuildPackage() rejected matching inputs: %v", err)
	}

	modifiedWasm := append([]byte(nil), wasmBytes...)
	modifiedWasm[len(modifiedWasm)-1] ^= 1
	if _, err := ValidateBuildPackage(manifestJSON, irJSON, modifiedWasm); err == nil {
		t.Fatal("ValidateBuildPackage() accepted a Wasm hash mismatch")
	}
	if _, err := ValidateBuildPackage(manifestJSON, append([]byte(" "), irJSON...), wasmBytes); err == nil {
		t.Fatal("ValidateBuildPackage() accepted non-canonical IR JSON")
	}
	unknownManifest := append([]byte(nil), manifestJSON[:len(manifestJSON)-1]...)
	unknownManifest = append(unknownManifest, []byte(`,"unreviewed":true}`)...)
	if _, err := ValidateBuildPackage(unknownManifest, irJSON, wasmBytes); err == nil {
		t.Fatal("ValidateBuildPackage() accepted an unknown manifest field")
	}
	unknownIR := append([]byte(nil), irJSON[:len(irJSON)-1]...)
	unknownIR = append(unknownIR, []byte(`,"unreviewed":true}`)...)
	if _, err := ValidateBuildPackage(manifestJSON, unknownIR, wasmBytes); err == nil {
		t.Fatal("ValidateBuildPackage() accepted an unknown IR field")
	}
}

func TestValidateBuildPackageAcceptsDynamicGuestSignature(t *testing.T) {
	ir := testDynamicCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	irJSON, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBuildPackage(manifestJSON, irJSON, wasmBytes); err != nil {
		t.Fatalf("ValidateBuildPackage() rejected a valid dynamic guest: %v", err)
	}
}

func TestValidateBuildPackageRejectsIncompatibleGuestSignature(t *testing.T) {
	ir := testCatalogIR()
	irBytes, err := json.Marshal(ir)
	if err != nil {
		t.Fatal(err)
	}
	wasmBytes, err := wasmtime.Wat2Wasm(`(module
  (import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
  (memory (export "memory") 17)
  (func (export "run") (result i64) (i64.const 0))
)`)
	if err != nil {
		t.Fatal(err)
	}
	irHash := sha256.Sum256(irBytes)
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmIRHashSection, []byte(hex.EncodeToString(irHash[:])))
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmCompilerSection, []byte(buildmeta.CurrentWASMCompiler))
	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBuildPackage(manifestJSON, irBytes, wasmBytes); err == nil {
		t.Fatal("ValidateBuildPackage() accepted an incompatible run signature")
	}
}

func TestValidateBuildPackageRejectsAdditionalWasmImport(t *testing.T) {
	ir := testCatalogIR()
	irBytes, err := json.Marshal(ir)
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
	irHash := sha256.Sum256(irBytes)
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmIRHashSection, []byte(hex.EncodeToString(irHash[:])))
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmCompilerSection, []byte(buildmeta.CurrentWASMCompiler))
	manifest, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateBuildPackage(manifestJSON, irBytes, wasmBytes); err == nil {
		t.Fatal("ValidateBuildPackage() accepted an additional WASI import")
	}
}
