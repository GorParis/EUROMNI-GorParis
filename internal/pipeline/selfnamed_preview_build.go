package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"euromni/internal/buildmeta"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

const (
	selfnamedPreviewGuestCompiler   = buildmeta.SelfnamedPreviewCompiler
	selfnamedPreviewCompilerSection = "euromni.compiler"
)

// CompileSelfnamedPreviewIRToWasm builds the approved Selfnamed guest and binds its exact IR hash.
func CompileSelfnamedPreviewIRToWasm(ir ModuleIR) ([]byte, error) {
	if _, err := ValidateIR(ir); err != nil {
		return nil, fmt.Errorf("validate Selfnamed preview IR: %w", err)
	}
	if !isSelfnamedPreviewIR(ir) {
		return nil, errors.New("IR does not match the Selfnamed preview guest contract")
	}
	guestDirectory, err := selfnamedPreviewGuestDirectory()
	if err != nil {
		return nil, err
	}
	for _, tool := range []struct {
		name   string
		prefix string
	}{
		{name: "rustc", prefix: "rustc 1.99.1 "},
		{name: "cargo", prefix: "cargo 1.99.1 "},
	} {
		versionCommand := exec.Command(tool.name, "--version")
		versionCommand.Dir = guestDirectory
		versionOutput, err := versionCommand.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("check pinned %s: %w: %s", tool.name, err, strings.TrimSpace(string(versionOutput)))
		}
		if !strings.HasPrefix(string(versionOutput), tool.prefix) {
			return nil, fmt.Errorf("unsupported %s version %q", tool.name, strings.TrimSpace(string(versionOutput)))
		}
	}
	targetDirectory, err := os.MkdirTemp("", "euromni-selfnamed-preview-build-*")
	if err != nil {
		return nil, fmt.Errorf("create temporary Rust build directory: %w", err)
	}
	defer os.RemoveAll(targetDirectory)

	command := exec.Command(
		"cargo",
		"build",
		"--frozen",
		"--release",
		"--target", "wasm32-unknown-unknown",
		"--target-dir", targetDirectory,
	)
	command.Dir = guestDirectory
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build Selfnamed preview guest: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	wasmPath := filepath.Join(targetDirectory, "wasm32-unknown-unknown", "release", "euromni_selfnamed_preview_guest.wasm")
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("read Selfnamed preview guest: %w", err)
	}
	irBytes, err := json.Marshal(ir)
	if err != nil {
		return nil, fmt.Errorf("encode Selfnamed preview IR: %w", err)
	}
	irHash := sha256.Sum256(irBytes)
	wasmBytes = appendWASMCustomSection(wasmBytes, wasmIRHashSection, []byte(hex.EncodeToString(irHash[:])))
	wasmBytes = appendWASMCustomSection(wasmBytes, selfnamedPreviewCompilerSection, []byte(selfnamedPreviewGuestCompiler))
	if err := validateSelfnamedPreviewGuestWasm(wasmBytes); err != nil {
		return nil, err
	}
	return wasmBytes, nil
}

func isSelfnamedPreviewIR(ir ModuleIR) bool {
	if ir.Module != "selfnamed-catalog-preview" ||
		ir.Platform != "woocommerce" ||
		len(ir.Triggers) != 1 ||
		len(ir.Operations) != 1 {
		return false
	}
	trigger := ir.Triggers[0]
	operation := ir.Operations[0]
	input, exists := operation.Input["product_json"]
	return trigger.Kind == "manual" &&
		trigger.Name == "supplier_product.preview" &&
		len(trigger.Fields) == 1 &&
		trigger.Fields["product_json"] == "string" &&
		operation.Call == "catalog.supplier.product.preview" &&
		len(operation.Input) == 1 &&
		exists &&
		input.TriggerField == "product_json" &&
		len(input.Literal) == 0
}

func selfnamedPreviewGuestDirectory() (string, error) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolve Selfnamed guest source location")
	}
	return filepath.Abs(filepath.Join(filepath.Dir(sourceFile), "..", "..", "pipeline", "testdata", "rust-selfnamed-preview"))
}

func validateSelfnamedPreviewGuestWasm(wasmBytes []byte) error {
	engine := wasmtime.NewEngine()
	defer engine.Close()
	module, err := wasmtime.NewModule(engine, wasmBytes)
	if err != nil {
		return fmt.Errorf("validate compiled Selfnamed preview guest: %w", err)
	}
	defer module.Close()
	imports := module.Imports()
	if len(imports) != 1 {
		for _, imported := range imports {
			imported.Close()
		}
		return fmt.Errorf("Selfnamed preview guest has %d imports, want only euromni.call", len(imports))
	}
	moduleName := imports[0].Module()
	importName := imports[0].Name()
	importedName := ""
	if importName != nil {
		importedName = *importName
	}
	imports[0].Close()
	if moduleName != "euromni" || importedName != "call" {
		return fmt.Errorf("Selfnamed preview guest imports %s.%s, want euromni.call", moduleName, importedName)
	}
	exports := make(map[string]struct{})
	for _, exported := range module.Exports() {
		exports[exported.Name()] = struct{}{}
		exported.Close()
	}
	if len(exports) != 2 {
		return fmt.Errorf("Selfnamed preview guest has %d exports, want memory and run", len(exports))
	}
	for _, name := range []string{"memory", "run"} {
		if _, exists := exports[name]; !exists {
			return fmt.Errorf("Selfnamed preview guest is missing %q export", name)
		}
	}
	return nil
}
