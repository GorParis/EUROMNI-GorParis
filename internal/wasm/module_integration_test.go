//go:build integration

package wasm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"euromni/internal/abi"
	"euromni/internal/buildmeta"
	"euromni/internal/seed"
	"euromni/internal/store"
	"euromni/internal/testdb"
)

func TestRuntimeLoadsDatabaseInstalledModule(t *testing.T) {
	ctx := context.Background()
	sqlDB := testdb.Start(t)
	moduleStore := store.NewSQL(sqlDB)
	host := testHost()
	trustedContext := mustCtx(t, CapCatalog)
	input := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.get","input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}}`)
	expected := host.Invoke(ctx, trustedContext, input)
	responseOffset := bytes.Index(expected, []byte(`"cabas-lin"`))
	if responseOffset < 0 {
		t.Fatalf("expected catalog response, got %s", expected)
	}
	wasm, err := catalogCallModule(input, responseOffset)
	if err != nil {
		t.Fatalf("compile test guest: %v", err)
	}
	wasmHash := sha256.Sum256(wasm)
	if err := moduleStore.RegisterModuleBuild(
		ctx, seed.ModuleID, wasmHash[:], testInstalledBuildManifest(t, wasm, "initial"),
	); err != nil {
		t.Fatalf("register guest hash: %v", err)
	}

	runtime := NewRuntime(host)
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, wasm); err != nil {
		t.Fatalf("load installed guest: %v", err)
	}
	results, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run")
	if err != nil {
		t.Fatalf("run installed guest: %v", err)
	}
	if len(results) != 1 || results[0] != int32(expected[responseOffset]) {
		t.Fatalf("guest result = %v, want response byte %d", results, expected[responseOffset])
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE shop_modules SET capabilities = ARRAY['customer'] WHERE shop_id::text = $1 AND module_id::text = $2`, seed.ShopID, seed.ModuleID); err != nil {
		t.Fatalf("revoke catalog capability: %v", err)
	}
	results, err = runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run")
	if err != nil {
		t.Fatalf("run after capability revocation: %v", err)
	}
	if len(results) != 1 || results[0] != int32(0) {
		t.Fatalf("result after capability revocation = %v, want denied response", results)
	}
	changedWASM := []byte("a different registered guest")
	changedHash := sha256.Sum256(changedWASM)
	if err := moduleStore.RegisterModuleBuild(
		ctx, seed.ModuleID, changedHash[:], testInstalledBuildManifest(t, changedWASM, "changed"),
	); err != nil {
		t.Fatalf("register changed module build: %v", err)
	}
	if err := moduleStore.PromoteInstalledModuleBuild(ctx, seed.ShopID, seed.ModuleID, hex.EncodeToString(changedHash[:])); err != nil {
		t.Fatalf("promote changed module build: %v", err)
	}
	if _, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run"); !errors.Is(err, ErrModuleNeedsReload) {
		t.Fatalf("run after hash change error = %v, want %v", err, ErrModuleNeedsReload)
	}
	if err := moduleStore.RegisterModuleBuild(
		ctx, seed.ModuleID, wasmHash[:], testInstalledBuildManifest(t, wasm, "initial"),
	); err != nil {
		t.Fatalf("republish original module build: %v", err)
	}
	if err := moduleStore.PromoteInstalledModuleBuild(ctx, seed.ShopID, seed.ModuleID, hex.EncodeToString(wasmHash[:])); err != nil {
		t.Fatalf("restore installed module build: %v", err)
	}
	if err := runtime.ReloadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, wasm); err != nil {
		t.Fatalf("reload changed module: %v", err)
	}
	if _, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run"); err != nil {
		t.Fatalf("run after reload: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE modules SET abi_version = 'euromni.abi.v0' WHERE id::text = $1`, seed.ModuleID); err != nil {
		t.Fatalf("change registered module ABI: %v", err)
	}
	if _, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run"); !errors.Is(err, ErrUnsupportedABI) {
		t.Fatalf("run after ABI change error = %v, want %v", err, ErrUnsupportedABI)
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE shop_modules SET status = 'disabled' WHERE shop_id::text = $1 AND module_id::text = $2`, seed.ShopID, seed.ModuleID); err != nil {
		t.Fatalf("disable module: %v", err)
	}
	if _, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run"); !errors.Is(err, ErrModuleNotInstalled) {
		t.Fatalf("run after disable error = %v, want %v", err, ErrModuleNotInstalled)
	}
}

func testInstalledBuildManifest(t *testing.T, wasm []byte, provenance string) []byte {
	t.Helper()
	digest := func(value string) string {
		hash := sha256.Sum256([]byte(value))
		return hex.EncodeToString(hash[:])
	}
	wasmHash := sha256.Sum256(wasm)
	manifest := map[string]any{
		"version":      buildmeta.ManifestVersion,
		"module":       "demo.loyalty",
		"platform":     buildmeta.PrestaShopAdapter,
		"ir_version":   buildmeta.IRVersion,
		"ir_sha256":    digest("ir-" + provenance),
		"abi_version":  abi.Version,
		"compiler":     buildmeta.CurrentWASMCompiler,
		"capabilities": []string{"catalog"},
		"wasm_sha256":  hex.EncodeToString(wasmHash[:]),
		"wasm_size":    len(wasm),
		"source": map[string]string{
			"source_sha256":    digest("source-" + provenance),
			"archive_sha256":   digest("archive-" + provenance),
			"module_version":   "1.0.0",
			"platform_version": "9.0.0",
			"adapter":          buildmeta.PrestaShopAdapter,
			"adapter_version":  buildmeta.PrestaShopAdapterVersion,
		},
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode module build manifest: %v", err)
	}
	return encoded
}
