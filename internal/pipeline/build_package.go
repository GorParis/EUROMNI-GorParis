package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

// ValidateBuildPackage validates the transport-independent manifest, canonical IR, and Wasm artifact.
func ValidateBuildPackage(manifestJSON, irJSON, wasmBytes []byte) (BuildManifest, error) {
	var manifest BuildManifest
	if err := decodeStrictJSON(manifestJSON, &manifest); err != nil {
		return BuildManifest{}, fmt.Errorf("decode build manifest: %w", err)
	}
	var ir ModuleIR
	if err := decodeStrictJSON(irJSON, &ir); err != nil {
		return BuildManifest{}, fmt.Errorf("decode build IR: %w", err)
	}
	canonicalIR, err := json.Marshal(ir)
	if err != nil {
		return BuildManifest{}, fmt.Errorf("encode canonical build IR: %w", err)
	}
	if !bytes.Equal(irJSON, canonicalIR) {
		return BuildManifest{}, errors.New("build IR is not in canonical JSON form")
	}
	if err := manifest.Validate(ir, wasmBytes); err != nil {
		return BuildManifest{}, fmt.Errorf("validate build manifest: %w", err)
	}
	if err := validateBuildWasmCompatibility(ir, wasmBytes); err != nil {
		return BuildManifest{}, err
	}
	return manifest, nil
}

func decodeStrictJSON(raw []byte, target any) error {
	if len(raw) == 0 {
		return errors.New("JSON document is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("JSON document has trailing data")
	}
	return nil
}

func validateBuildWasmCompatibility(ir ModuleIR, wasmBytes []byte) error {
	engine := wasmtime.NewEngine()
	defer engine.Close()
	module, err := wasmtime.NewModule(engine, wasmBytes)
	if err != nil {
		return fmt.Errorf("compile distributable Wasm artifact: %w", err)
	}
	defer module.Close()

	imports := module.Imports()
	if len(imports) != 1 {
		for _, imported := range imports {
			imported.Close()
		}
		return fmt.Errorf("distributable Wasm has %d imports, want only euromni.call", len(imports))
	}
	imported := imports[0]
	importModule := imported.Module()
	importName := imported.Name()
	importType := imported.Type()
	imported.Close()
	if importName == nil || importModule != "euromni" || *importName != "call" {
		importType.Close()
		return fmt.Errorf("distributable Wasm import is %s.%v, want euromni.call", importModule, importName)
	}
	callType := importType.FuncType()
	if callType == nil || !wasmFuncSignatureMatches(callType, []wasmtime.ValKind{
		wasmtime.KindI32, wasmtime.KindI32, wasmtime.KindI32, wasmtime.KindI32,
	}, []wasmtime.ValKind{wasmtime.KindI32}) {
		importType.Close()
		return errors.New("distributable Wasm euromni.call has an incompatible signature")
	}
	importType.Close()

	exports := module.Exports()
	if len(exports) != 2 {
		for _, exported := range exports {
			exported.Close()
		}
		return fmt.Errorf("distributable Wasm has %d exports, want exactly memory and run", len(exports))
	}
	dynamicInput := operationHasDynamicInput(ir.Operations[0])
	seenMemory, seenRun := false, false
	for _, exported := range exports {
		name := exported.Name()
		exportType := exported.Type()
		exported.Close()
		switch name {
		case "memory":
			memoryType := exportType.MemoryType()
			if memoryType == nil {
				exportType.Close()
				return errors.New("distributable Wasm memory export has an incompatible type")
			}
			maximumPresent, maximum := memoryType.Maximum()
			if memoryType.Is64() || memoryType.IsShared() || memoryType.Minimum() == 0 ||
				memoryType.Minimum() > guestMemoryLimitPages ||
				(maximumPresent && (maximum < memoryType.Minimum() || maximum > guestMemoryLimitPages)) {
				exportType.Close()
				return errors.New("distributable Wasm memory limits are incompatible")
			}
			seenMemory = true
		case "run":
			runType := exportType.FuncType()
			params := []wasmtime.ValKind{}
			if dynamicInput {
				params = []wasmtime.ValKind{wasmtime.KindI32, wasmtime.KindI32}
			}
			if runType == nil || !wasmFuncSignatureMatches(runType, params, []wasmtime.ValKind{wasmtime.KindI32}) {
				exportType.Close()
				return errors.New("distributable Wasm run export has an incompatible signature")
			}
			seenRun = true
		default:
			exportType.Close()
			return fmt.Errorf("distributable Wasm has unexpected export %q", name)
		}
		exportType.Close()
	}
	if !seenMemory || !seenRun {
		return errors.New("distributable Wasm is missing memory or run export")
	}
	return nil
}

const guestMemoryLimitPages = 64 << 10

func wasmFuncSignatureMatches(functionType *wasmtime.FuncType, params, results []wasmtime.ValKind) bool {
	actualParams := functionType.Params()
	actualResults := functionType.Results()
	if len(actualParams) != len(params) || len(actualResults) != len(results) {
		return false
	}
	for index, parameter := range actualParams {
		if parameter.Kind() != params[index] {
			return false
		}
	}
	for index, result := range actualResults {
		if result.Kind() != results[index] {
			return false
		}
	}
	return true
}
