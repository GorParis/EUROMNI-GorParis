package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"euromni/internal/abi"
	"euromni/internal/buildmeta"
)

const BuildManifestVersion = buildmeta.ManifestVersion
const IRCompilerVersion = buildmeta.CurrentWASMCompiler

type BuildManifest struct {
	Version      string           `json:"version"`
	Module       string           `json:"module"`
	Platform     string           `json:"platform"`
	Source       SourceProvenance `json:"source"`
	IRVersion    string           `json:"ir_version"`
	IRSHA256     string           `json:"ir_sha256"`
	ABIVersion   string           `json:"abi_version"`
	Compiler     string           `json:"compiler"`
	Capabilities []string         `json:"capabilities"`
	WASMSHA256   string           `json:"wasm_sha256"`
	WASMSize     int64            `json:"wasm_size"`
}

func NewBuildManifest(ir ModuleIR, wasmBytes []byte) (BuildManifest, error) {
	validated, err := ValidateIR(ir)
	if err != nil {
		return BuildManifest{}, fmt.Errorf("validate IR for build manifest: %w", err)
	}
	if len(wasmBytes) == 0 {
		return BuildManifest{}, errors.New("Wasm bytes are required for build manifest")
	}
	irBytes, err := json.Marshal(ir)
	if err != nil {
		return BuildManifest{}, fmt.Errorf("encode IR for build manifest: %w", err)
	}
	irHash := sha256.Sum256(irBytes)
	wasmHash := sha256.Sum256(wasmBytes)
	capabilities := append([]string(nil), validated.Capabilities...)
	sort.Strings(capabilities)
	return BuildManifest{
		Version:      BuildManifestVersion,
		Module:       ir.Module,
		Platform:     ir.Platform,
		Source:       ir.Provenance,
		IRVersion:    ir.Version,
		IRSHA256:     hex.EncodeToString(irHash[:]),
		ABIVersion:   abi.Version,
		Compiler:     IRCompilerVersion,
		Capabilities: capabilities,
		WASMSHA256:   hex.EncodeToString(wasmHash[:]),
		WASMSize:     int64(len(wasmBytes)),
	}, nil
}

func (manifest BuildManifest) Validate(ir ModuleIR, wasmBytes []byte) error {
	expected, err := NewBuildManifest(ir, wasmBytes)
	if err != nil {
		return err
	}
	if manifest.Version != expected.Version ||
		manifest.Module != expected.Module ||
		manifest.Platform != expected.Platform ||
		manifest.Source != expected.Source ||
		manifest.IRVersion != expected.IRVersion ||
		manifest.IRSHA256 != expected.IRSHA256 ||
		manifest.ABIVersion != expected.ABIVersion ||
		manifest.Compiler != expected.Compiler ||
		manifest.WASMSHA256 != expected.WASMSHA256 ||
		manifest.WASMSize != expected.WASMSize ||
		!equalStrings(manifest.Capabilities, expected.Capabilities) {
		return errors.New("build manifest does not match IR and Wasm artifact")
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
