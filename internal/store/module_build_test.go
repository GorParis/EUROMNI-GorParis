package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"euromni/internal/abi"
	"euromni/internal/buildmeta"
)

func TestValidateBuildManifestJSON(t *testing.T) {
	wasm := []byte("test wasm")
	digest := sha256.Sum256(wasm)
	valid := testBuildManifestJSON(wasm, "valid")
	if err := validateBuildManifestJSON(digest[:], valid); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	var validObject map[string]any
	if err := json.Unmarshal(valid, &validObject); err != nil {
		t.Fatal(err)
	}
	legacyCompiler, err := json.Marshal(func() map[string]any {
		copy := cloneManifestObject(validObject)
		copy["compiler"] = buildmeta.LegacyWASMCompiler
		return copy
	}())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateBuildManifestJSON(digest[:], legacyCompiler); err != nil {
		t.Fatalf("legacy compiler manifest rejected: %v", err)
	}
	unsupportedCompiler, err := json.Marshal(func() map[string]any {
		copy := cloneManifestObject(validObject)
		copy["compiler"] = "wasmtime-go/latest"
		return copy
	}())
	if err != nil {
		t.Fatal(err)
	}
	unsupportedAdapter, err := json.Marshal(func() map[string]any {
		copy := cloneManifestObject(validObject)
		source := cloneManifestObject(copy["source"].(map[string]any))
		source["adapter_version"] = "9.9.9"
		copy["source"] = source
		return copy
	}())
	if err != nil {
		t.Fatal(err)
	}
	unsupportedIR, err := json.Marshal(func() map[string]any {
		copy := cloneManifestObject(validObject)
		copy["ir_version"] = "euromni.ir.next"
		return copy
	}())
	if err != nil {
		t.Fatal(err)
	}
	missingCompiler, err := json.Marshal(func() map[string]any {
		copy := cloneManifestObject(validObject)
		delete(copy, "compiler")
		return copy
	}())
	if err != nil {
		t.Fatal(err)
	}
	unknownField, err := json.Marshal(func() map[string]any {
		copy := cloneManifestObject(validObject)
		copy["unexpected"] = "field"
		return copy
	}())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		hash []byte
		json []byte
	}{
		{name: "short digest", hash: digest[:3], json: valid},
		{name: "malformed JSON", hash: digest[:], json: []byte(`{"wasm_sha256":`)},
		{name: "trailing JSON", hash: digest[:], json: append(append([]byte(nil), valid...), []byte(` {}`)...)},
		{name: "wrong Wasm hash", hash: make([]byte, sha256.Size), json: valid},
		{name: "missing compiler", hash: digest[:], json: missingCompiler},
		{name: "unknown manifest field", hash: digest[:], json: unknownField},
		{name: "unsupported compiler", hash: digest[:], json: unsupportedCompiler},
		{name: "unsupported adapter", hash: digest[:], json: unsupportedAdapter},
		{name: "unsupported IR version", hash: digest[:], json: unsupportedIR},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateBuildManifestJSON(test.hash, test.json); !errors.Is(err, ErrInvalidBuildManifest) {
				t.Fatalf("validation error = %v, want ErrInvalidBuildManifest", err)
			}
		})
	}
}

func cloneManifestObject(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func testBuildManifestJSON(wasm []byte, provenance string) []byte {
	sourceHash := sha256.Sum256([]byte("source-" + provenance))
	archiveHash := sha256.Sum256([]byte("archive-" + provenance))
	irHash := sha256.Sum256([]byte("ir-" + provenance))
	wasmHash := sha256.Sum256(wasm)
	manifest := struct {
		Version      string            `json:"version"`
		Module       string            `json:"module"`
		Platform     string            `json:"platform"`
		Source       map[string]string `json:"source"`
		IRVersion    string            `json:"ir_version"`
		IRSHA256     string            `json:"ir_sha256"`
		ABIVersion   string            `json:"abi_version"`
		Compiler     string            `json:"compiler"`
		Capabilities []string          `json:"capabilities"`
		WASMSHA256   string            `json:"wasm_sha256"`
		WASMSize     int               `json:"wasm_size"`
	}{
		Version:      "euromni.build.v1",
		Module:       "test.module",
		Platform:     "prestashop",
		Source:       map[string]string{"source_sha256": hex.EncodeToString(sourceHash[:]), "archive_sha256": hex.EncodeToString(archiveHash[:]), "platform_version": "9.0.0", "adapter": "prestashop", "adapter_version": "1.0.0", "module_version": "1.0.0"},
		IRVersion:    "euromni.ir.v3",
		IRSHA256:     hex.EncodeToString(irHash[:]),
		ABIVersion:   abi.Version,
		Compiler:     buildmeta.CurrentWASMCompiler,
		Capabilities: []string{"catalog"},
		WASMSHA256:   hex.EncodeToString(wasmHash[:]),
		WASMSize:     len(wasm),
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		panic(err)
	}
	return encoded
}
