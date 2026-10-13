package wasm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"

	"euromni/internal/abi"
	"euromni/internal/seed"
	"euromni/internal/store"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

func TestRuntimeLoadsAndClosesCompiledModule(t *testing.T) {
	runtime := New()
	t.Cleanup(runtime.Close)

	wasm, err := wasmtime.Wat2Wasm(`(module (func (export "run")))`)
	if err != nil {
		t.Fatalf("compile test module: %v", err)
	}
	if err := runtime.Load("test-module", wasm); err != nil {
		t.Fatalf("load module: %v", err)
	}
	if runtime.modules[manualModuleKey("test-module")] == nil {
		t.Fatal("compiled module was not registered")
	}
	if _, err := runtime.Run(context.Background(), nil, seed.ShopID, seed.ModuleID, "run"); !errors.Is(err, ErrModuleNotInstalled) {
		t.Fatalf("unregistered module run error = %v, want %v", err, ErrModuleNotInstalled)
	}
	if err := runtime.Load("test-module", wasm); !errors.Is(err, ErrModuleAlreadyLoaded) {
		t.Fatalf("duplicate load error = %v, want %v", err, ErrModuleAlreadyLoaded)
	}

	runtime.Close()
	if err := runtime.Load("after-close", wasm); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("load after close error = %v, want %v", err, ErrRuntimeClosed)
	}
}

func TestLoadInstalledRejectsUntrustedMetadata(t *testing.T) {
	wasm, err := wasmtime.Wat2Wasm(`(module (func (export "run")))`)
	if err != nil {
		t.Fatalf("compile test module: %v", err)
	}
	wasmHash := sha256.Sum256(wasm)
	cases := []struct {
		name   string
		change func(*store.InstalledModule)
		want   error
	}{
		{name: "missing hash", change: func(module *store.InstalledModule) { module.WASMSHA256 = nil }, want: ErrModuleHashMissing},
		{name: "wrong hash", change: func(module *store.InstalledModule) { module.WASMSHA256 = make([]byte, sha256.Size) }, want: ErrModuleHashMismatch},
		{name: "wrong ABI", change: func(module *store.InstalledModule) { module.ABIVersion = "euromni.abi.v0" }, want: ErrUnsupportedABI},
		{name: "unknown capability", change: func(module *store.InstalledModule) { module.Capabilities = []string{"filesystem"} }, want: ErrUnknownCapability},
		{name: "wrong module id", change: func(module *store.InstalledModule) { module.ID = "e0e00000-0001-4000-8000-000000000099" }, want: ErrModuleNotInstalled},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			module := &store.InstalledModule{
				ID: seed.ModuleID, Name: "test-module", ABIVersion: abi.Version,
				WASMSHA256: wasmHash[:], Capabilities: []string{"catalog"},
			}
			testCase.change(module)
			runtime := New()
			t.Cleanup(runtime.Close)
			err := runtime.loadInstalledRecord(module, seed.ShopID, seed.ModuleID, wasm)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("load error = %v, want %v", err, testCase.want)
			}
			if _, err := runtime.Run(context.Background(), nil, seed.ShopID, seed.ModuleID, "run"); !errors.Is(err, ErrModuleNotInstalled) {
				t.Fatalf("run after rejected load = %v, want %v", err, ErrModuleNotInstalled)
			}
		})
	}
}

func TestRuntimeRejectsInvalidModulesAndNames(t *testing.T) {
	runtime := New()
	t.Cleanup(runtime.Close)

	if err := runtime.Load("", []byte{0, 97, 115, 109}); err == nil {
		t.Fatal("empty module name was accepted")
	}
	if err := runtime.Load("bad-module", []byte("not wasm")); err == nil {
		t.Fatal("invalid module bytes were accepted")
	}
}

func TestRuntimeCallsHostThroughLinearMemory(t *testing.T) {
	host := testHost()
	runtime := NewRuntime(host)
	t.Cleanup(runtime.Close)
	trustedContext := mustCtx(t, CapCatalog)
	input := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.get","input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}}`)
	expected := host.Invoke(context.Background(), trustedContext, input)
	responseOffset := bytes.Index(expected, []byte(`"cabas-lin"`))
	if responseOffset < 0 {
		t.Fatalf("expected host response to contain product slug: %s", expected)
	}

	wasm, err := catalogCallModule(input, responseOffset)
	if err != nil {
		t.Fatalf("compile ABI guest: %v", err)
	}
	wasmHash := sha256.Sum256(wasm)
	installed := &store.InstalledModule{
		ID: seed.ModuleID, Name: "catalog-reader", ABIVersion: abi.Version,
		WASMSHA256: wasmHash[:], Capabilities: []string{"catalog"},
	}
	if err := runtime.loadInstalledRecord(installed, seed.ShopID, seed.ModuleID, wasm); err != nil {
		t.Fatalf("load installed ABI guest: %v", err)
	}
	results, err := runtime.run(context.Background(), trustedContext, installedModuleKey(seed.ShopID, seed.ModuleID), "run")
	if err != nil {
		t.Fatalf("run ABI guest: %v", err)
	}
	if len(results) != 1 || results[0] != int32(expected[responseOffset]) {
		t.Fatalf("guest result = %v, want response byte %d", results, expected[responseOffset])
	}
}

func TestRuntimeCallsHostWithDynamicGuestInput(t *testing.T) {
	host := testHost()
	runtime := NewRuntime(host)
	t.Cleanup(runtime.Close)
	trustedContext := mustCtx(t, CapCatalog)
	input := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.get","input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}}`)
	expected := host.Invoke(context.Background(), trustedContext, input)
	wasm, err := wasmtime.Wat2Wasm(`(module
		(import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
		(memory (export "memory") 17)
		(func (export "run") (param i32 i32) (result i32)
			(call $call (local.get 0) (local.get 1) (i32.const 16384) (i32.const 1048576))))`)
	if err != nil {
		t.Fatalf("compile dynamic-input guest: %v", err)
	}
	if err := runtime.Load("dynamic-input", wasm); err != nil {
		t.Fatalf("load dynamic-input guest: %v", err)
	}
	results, err := runtime.runWithInput(context.Background(), trustedContext, manualModuleKey("dynamic-input"), "run", input)
	if err != nil {
		t.Fatalf("run dynamic-input guest: %v", err)
	}
	if len(results) != 1 || results[0] != int32(len(expected)) {
		t.Fatalf("guest result = %v, want host response size %d", results, len(expected))
	}
}

func TestValidateGuestInputRejectsInvalidEnvelopes(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "empty"},
		{name: "malformed", input: []byte(`not json`)},
		{name: "too large", input: bytes.Repeat([]byte("x"), abi.MaxCallBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateGuestInput(test.input); err == nil {
				t.Fatal("validateGuestInput() accepted invalid input")
			}
		})
	}
}

func TestRuntimeScopesCapabilitiesByShopAndModule(t *testing.T) {
	host := testHost()
	runtime := NewRuntime(host)
	t.Cleanup(runtime.Close)
	input := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.get","input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}}`)
	responseWithoutCapability := host.Invoke(context.Background(), TrustedContext{}, input)
	if bytes.Contains(responseWithoutCapability, []byte(`"cabas-lin"`)) {
		t.Fatal("response without catalog capability unexpectedly contains product data")
	}
	expectedResponse := host.Invoke(context.Background(), mustCtx(t, CapCatalog), input)
	responseOffset := bytes.Index(expectedResponse, []byte(`"cabas-lin"`))
	if responseOffset < 0 || len(responseWithoutCapability) > responseOffset {
		t.Fatalf("test responses do not distinguish the capability result")
	}
	wasm, err := catalogCallModule(input, responseOffset)
	if err != nil {
		t.Fatalf("compile guest: %v", err)
	}
	wasmHash := sha256.Sum256(wasm)
	installed := &store.InstalledModule{
		ID: seed.ModuleID, Name: "demo.loyalty", ABIVersion: abi.Version,
		WASMSHA256: wasmHash[:], Capabilities: []string{"catalog"},
	}
	if err := runtime.loadInstalledRecord(installed, seed.ShopID, seed.ModuleID, wasm); err != nil {
		t.Fatalf("install first shop module: %v", err)
	}
	otherShopModule := *installed
	otherShopModule.Capabilities = nil
	if err := runtime.loadInstalledRecord(&otherShopModule, seed.OtherShop, seed.ModuleID, wasm); err != nil {
		t.Fatalf("install second shop module: %v", err)
	}

	firstResults, err := runtime.run(context.Background(), mustCtx(t, CapCatalog), installedModuleKey(seed.ShopID, seed.ModuleID), "run")
	if err != nil {
		t.Fatalf("run first shop module: %v", err)
	}
	if len(firstResults) != 1 || firstResults[0] != int32(expectedResponse[responseOffset]) {
		t.Fatalf("first shop result = %v, want catalog response", firstResults)
	}
	secondResults, err := runtime.run(context.Background(), TrustedContext{}, installedModuleKey(seed.OtherShop, seed.ModuleID), "run")
	if err != nil {
		t.Fatalf("run second shop module: %v", err)
	}
	if len(secondResults) != 1 || secondResults[0] != int32(0) {
		t.Fatalf("second shop result = %v, want denied response without catalog data", secondResults)
	}
}

func TestRuntimeRejectsOutOfBoundsGuestMemory(t *testing.T) {
	runtime := NewRuntime(testHost())
	t.Cleanup(runtime.Close)
	wasm, err := wasmtime.Wat2Wasm(`(module
		(import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
		(memory (export "memory") 1)
		(func (export "run") (result i32)
			(call $call (i32.const 65535) (i32.const 2) (i32.const 0) (i32.const 16))))`)
	if err != nil {
		t.Fatalf("compile bounds guest: %v", err)
	}
	if err := runtime.Load("bounds-check", wasm); err != nil {
		t.Fatalf("load bounds guest: %v", err)
	}
	results, err := runtime.runWithContext(context.Background(), mustCtx(t, CapCatalog), "bounds-check", "run")
	if err != nil {
		t.Fatalf("run bounds guest: %v", err)
	}
	if len(results) != 1 || results[0] != int32(-1) {
		t.Fatalf("bounds result = %v, want transport error -1", results)
	}
}

func TestRuntimeReturnsABISizeErrorForOversizedInput(t *testing.T) {
	host := testHost()
	runtime := NewRuntime(host)
	t.Cleanup(runtime.Close)
	trustedContext := mustCtx(t, CapCatalog)
	input := bytes.Repeat([]byte("x"), 16<<10|1)
	expected := host.Invoke(context.Background(), trustedContext, input)
	responseOffset := bytes.Index(expected, []byte(`"too_large"`))
	if responseOffset < 0 {
		t.Fatalf("expected too_large response, got %s", expected)
	}
	wasm, err := catalogCallModule(input, responseOffset)
	if err != nil {
		t.Fatalf("compile oversized guest: %v", err)
	}
	if err := runtime.Load("oversized-input", wasm); err != nil {
		t.Fatalf("load oversized guest: %v", err)
	}
	results, err := runtime.runWithContext(context.Background(), trustedContext, "oversized-input", "run")
	if err != nil {
		t.Fatalf("run oversized guest: %v", err)
	}
	if len(results) != 1 || results[0] != int32(expected[responseOffset]) {
		t.Fatalf("guest result = %v, want too_large response byte %d", results, expected[responseOffset])
	}
}

func TestRuntimeRejectsUnlinkedImports(t *testing.T) {
	runtime := New()
	t.Cleanup(runtime.Close)
	wasm, err := wasmtime.Wat2Wasm(`(module
		(import "wasi_snapshot_preview1" "fd_write" (func))
		(func (export "run")))`)
	if err != nil {
		t.Fatalf("compile guest with unsupported import: %v", err)
	}
	if err := runtime.Load("unsupported-import", wasm); err != nil {
		t.Fatalf("load guest: %v", err)
	}
	if _, err := runtime.runWithContext(context.Background(), TrustedContext{}, "unsupported-import", "run"); err == nil {
		t.Fatal("unlinked WASI import was allowed")
	}
}

func TestRuntimeStopsGuestWhenFuelIsExhausted(t *testing.T) {
	runtime := New()
	t.Cleanup(runtime.Close)
	wasm, err := wasmtime.Wat2Wasm(`(module (func (export "run") (loop $forever (br $forever))))`)
	if err != nil {
		t.Fatalf("compile infinite guest: %v", err)
	}
	if err := runtime.Load("infinite-loop", wasm); err != nil {
		t.Fatalf("load infinite guest: %v", err)
	}
	if _, err := runtime.runWithContext(context.Background(), TrustedContext{}, "infinite-loop", "run"); err == nil {
		t.Fatal("infinite guest completed without exhausting fuel")
	}
}

func TestRuntimeEnforcesMemoryLimit(t *testing.T) {
	runtime := New()
	t.Cleanup(runtime.Close)
	wasm, err := wasmtime.Wat2Wasm(`(module (memory (export "memory") 1025) (func (export "run")))`)
	if err != nil {
		t.Fatalf("compile oversized-memory guest: %v", err)
	}
	if err := runtime.Load("oversized-memory", wasm); err != nil {
		t.Fatalf("load oversized-memory guest: %v", err)
	}
	if _, err := runtime.runWithContext(context.Background(), TrustedContext{}, "oversized-memory", "run"); err == nil {
		t.Fatal("guest memory above runtime limit was allowed")
	}
}

func escapeWAT(input []byte) string {
	var escaped strings.Builder
	for _, value := range input {
		_, _ = fmt.Fprintf(&escaped, "\\%02x", value)
	}
	return escaped.String()
}

func catalogCallModule(input []byte, responseOffset int) ([]byte, error) {
	const outputOffset = 32_768
	wat := fmt.Sprintf(`(module
		(import "euromni" "call" (func $call (param i32 i32 i32 i32) (result i32)))
		(memory (export "memory") 2)
		(data (i32.const 0) "%s")
		(func (export "run") (result i32)
			(drop (call $call (i32.const 0) (i32.const %d) (i32.const %d) (i32.const 32768)))
			(i32.load8_u (i32.const %d))))`, escapeWAT(input), len(input), outputOffset, outputOffset+responseOffset)
	return wasmtime.Wat2Wasm(wat)
}
