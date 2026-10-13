//go:build integration

package wasm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

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
	if _, err := sqlDB.ExecContext(ctx, `UPDATE modules SET wasm_sha256 = $2 WHERE id::text = $1`, seed.ModuleID, wasmHash[:]); err != nil {
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
	if _, err := sqlDB.ExecContext(ctx, `UPDATE modules SET wasm_sha256 = decode(repeat('00', 32), 'hex') WHERE id::text = $1`, seed.ModuleID); err != nil {
		t.Fatalf("change registered module hash: %v", err)
	}
	if _, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run"); !errors.Is(err, ErrModuleNeedsReload) {
		t.Fatalf("run after hash change error = %v, want %v", err, ErrModuleNeedsReload)
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE modules SET wasm_sha256 = $2 WHERE id::text = $1`, seed.ModuleID, wasmHash[:]); err != nil {
		t.Fatalf("restore registered module hash: %v", err)
	}
	if err := runtime.ReloadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, wasm); err != nil {
		t.Fatalf("reload changed module: %v", err)
	}
	if _, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run"); err != nil {
		t.Fatalf("run after reload: %v", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `UPDATE modules SET wasm_sha256 = $2, abi_version = 'euromni.abi.v0' WHERE id::text = $1`, seed.ModuleID, wasmHash[:]); err != nil {
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
