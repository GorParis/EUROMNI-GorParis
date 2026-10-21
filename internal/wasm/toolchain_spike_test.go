//go:build wasmtoolchain

package wasm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

func TestGoWASIP1ToolchainImportSet(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	output := filepath.Join(t.TempDir(), "go-wasip1.wasm")
	command := exec.Command("go", "build", "-trimpath", "-o", output, "./pipeline/testdata/go-wasip1")
	command.Dir = projectRoot
	command.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm", "CGO_ENABLED=0")
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Go WASI guest: %v\n%s", err, result)
	}
	wasm, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}

	engine := wasmtime.NewEngine()
	defer engine.Close()
	module, err := wasmtime.NewModule(engine, wasm)
	if err != nil {
		t.Fatalf("inspect Go WASI guest: %v", err)
	}
	defer module.Close()

	hasEUROMNICall := false
	imports := make([]string, 0)
	for _, imported := range module.Imports() {
		moduleName := imported.Module()
		name := imported.Name()
		importName := "<nil>"
		if name != nil {
			importName = *name
		}
		if moduleName == guestCallModule && importName == guestCallName {
			hasEUROMNICall = true
		} else {
			imports = append(imports, moduleName+"."+importName)
		}
		imported.Close()
	}
	if !hasEUROMNICall {
		t.Fatal("Go guest did not retain the custom euromni.call import")
	}
	if len(imports) == 0 {
		t.Fatal("Go toolchain import set changed; review the recorded QP-01 compatibility decision")
	}
	t.Logf("Go WASI guest requires imports beyond euromni.call: %s", strings.Join(imports, ", "))
}

func TestMinimalCatalogGuestRequiresOnlyEuromniCall(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	path := filepath.Join(projectRoot, "pipeline", "testdata", "minimal-catalog", "minimal_catalog.wat")
	wat, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read minimal catalog guest: %v", err)
	}
	wasm, err := wasmtime.Wat2Wasm(string(wat))
	if err != nil {
		t.Fatalf("compile minimal catalog guest: %v", err)
	}

	engine := wasmtime.NewEngine()
	defer engine.Close()
	module, err := wasmtime.NewModule(engine, wasm)
	if err != nil {
		t.Fatalf("inspect minimal catalog guest: %v", err)
	}
	defer module.Close()

	hasEUROMNICall := false
	imports := make([]string, 0)
	for _, imported := range module.Imports() {
		moduleName := imported.Module()
		name := imported.Name()
		importName := "<nil>"
		if name != nil {
			importName = *name
		}
		if moduleName == guestCallModule && importName == guestCallName {
			hasEUROMNICall = true
			continue
		}
		imports = append(imports, moduleName+"."+importName)
		imported.Close()
	}
	if !hasEUROMNICall {
		t.Fatal("minimal catalog guest did not retain the custom euromni.call import")
	}
	if len(imports) != 0 {
		t.Fatalf("minimal catalog guest imported unexpected symbols: %s", strings.Join(imports, ", "))
	}
	t.Log("minimal catalog guest uses only euromni.call and no WASI imports")
}

func TestRustUnknownUnknownGuestImportsOnlyEuromniCallAndExecutes(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	fixtureDirectory := filepath.Join(projectRoot, "pipeline", "testdata", "rust-euromni")
	buildDirectory := t.TempDir()
	rustVersion := commandVersion(t, fixtureDirectory, "rustc", "--version")
	cargoVersion := commandVersion(t, fixtureDirectory, "cargo", "--version")
	if !strings.HasPrefix(rustVersion, "rustc 1.99.1 ") {
		t.Fatalf("Rust toolchain = %q, want the pinned 1.99.1 toolchain", rustVersion)
	}
	build := func(targetDirectory string) []byte {
		t.Helper()
		command := exec.Command("cargo",
			"build",
			"--frozen",
			"--release",
			"--target", "wasm32-unknown-unknown",
			"--target-dir", targetDirectory,
		)
		command.Dir = fixtureDirectory
		if result, err := command.CombinedOutput(); err != nil {
			t.Fatalf("build locked Rust wasm32-unknown-unknown guest: %v\n%s", err, result)
		}
		wasmPath := filepath.Join(targetDirectory, "wasm32-unknown-unknown", "release", "euromni_rust_guest.wasm")
		wasm, err := os.ReadFile(wasmPath)
		if err != nil {
			t.Fatalf("read Rust guest: %v", err)
		}
		return wasm
	}
	wasm := build(filepath.Join(buildDirectory, "build-a"))
	repeatedWASM := build(filepath.Join(buildDirectory, "build-b"))
	if !bytes.Equal(wasm, repeatedWASM) {
		firstHash := sha256.Sum256(wasm)
		secondHash := sha256.Sum256(repeatedWASM)
		t.Fatalf("identical Rust guest builds produced different Wasm bytes: %s != %s",
			hex.EncodeToString(firstHash[:]), hex.EncodeToString(secondHash[:]))
	}
	wasmHash := sha256.Sum256(wasm)
	t.Logf("reproducible guest: %s; %s; target=wasm32-unknown-unknown; host=%s/%s; artifact_sha256=%s",
		rustVersion, cargoVersion, runtime.GOOS, runtime.GOARCH, hex.EncodeToString(wasmHash[:]))

	engine := wasmtime.NewEngine()
	defer engine.Close()
	module, err := wasmtime.NewModule(engine, wasm)
	if err != nil {
		t.Fatalf("inspect Rust guest: %v", err)
	}
	defer module.Close()

	imports := make([]string, 0)
	for _, imported := range module.Imports() {
		moduleName := imported.Module()
		name := imported.Name()
		importName := "<nil>"
		if name != nil {
			importName = *name
		}
		imports = append(imports, moduleName+"."+importName)
		imported.Close()
	}
	if len(imports) != 1 || imports[0] != guestCallModule+"."+guestCallName {
		t.Fatalf("Rust guest imports = %v, want only %s.%s", imports, guestCallModule, guestCallName)
	}
	exports := make(map[string]struct{})
	for _, exported := range module.Exports() {
		exports[exported.Name()] = struct{}{}
	}
	for _, name := range []string{"memory", "run"} {
		if _, ok := exports[name]; !ok {
			t.Errorf("Rust guest is missing required %q export", name)
		}
	}
	if t.Failed() {
		return
	}

	host := testHost()
	runtime := NewRuntime(host)
	defer runtime.Close()
	if err := runtime.Load("rust-minimal-catalog", wasm); err != nil {
		t.Fatalf("load Rust guest: %v", err)
	}
	input := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.get","input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}}`)
	expected := host.Invoke(context.Background(), mustCtx(t, CapCatalog), input)
	results, err := runtime.runWithInput(context.Background(), mustCtx(t, CapCatalog), manualModuleKey("rust-minimal-catalog"), "run", input)
	if err != nil {
		t.Fatalf("run Rust guest through EUROMNI ABI: %v", err)
	}
	if len(results) != 1 || results[0] != int32(len(expected)) {
		t.Fatalf("Rust guest result = %v, want ABI response size %d", results, len(expected))
	}
}

func commandVersion(t *testing.T, directory, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func TestMinimalCatalogGuestRuntimeExecution(t *testing.T) {
	host := testHost()
	runtime := NewRuntime(host)
	defer runtime.Close()

	input := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.get","input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}}`)
	expected := host.Invoke(context.Background(), mustCtx(t, CapCatalog), input)
	responseOffset := bytes.Index(expected, []byte(`"cabas-lin"`))
	if responseOffset < 0 {
		t.Fatalf("expected product response in host output: %s", expected)
	}

	const outputOffset = 32_768
	wat := fmt.Sprintf(`(module
		(import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
		(memory (export "memory") 2)
		(data (i32.const 0) "%s")
		(func (export "run") (result i32)
			(drop (call $call (i32.const 0) (i32.const %d) (i32.const %d) (i32.const %d)))
			(i32.load8_u (i32.const %d))))`, escapeWAT(input), len(input), outputOffset, outputOffset, outputOffset+responseOffset)
	wasm, err := wasmtime.Wat2Wasm(wat)
	if err != nil {
		t.Fatalf("compile runtime minimal catalog guest: %v", err)
	}
	if err := runtime.Load("minimal-catalog", wasm); err != nil {
		t.Fatalf("load runtime minimal catalog guest: %v", err)
	}
	results, err := runtime.runWithContext(context.Background(), mustCtx(t, CapCatalog), "minimal-catalog", "run")
	if err != nil {
		t.Fatalf("run runtime minimal catalog guest: %v", err)
	}
	if len(results) != 1 || int32(results[0].(int32)) != int32(expected[responseOffset]) {
		t.Fatalf("runtime result = %v, want response byte %d", results, expected[responseOffset])
	}

	resultsDenied, err := runtime.runWithContext(context.Background(), TrustedContext{}, "minimal-catalog", "run")
	if err != nil {
		t.Fatalf("run minimal catalog guest without capacity: %v", err)
	}
	if len(resultsDenied) != 1 || len(results) != 1 {
		t.Fatalf("runtime result counts = %d/%d, want exactly one value each", len(resultsDenied), len(results))
	}
	if resultsDenied[0] == results[0] {
		t.Fatalf("runtime result without catalog capability matched allowed result: %v == %v", resultsDenied[0], results[0])
	}
	t.Log("minimal catalog guest succeeds with catalog capability and does not return the authorized value without it")
}
