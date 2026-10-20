//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"euromni/internal/abi"
	"euromni/internal/artifact"
	"euromni/internal/seed"
	"euromni/internal/store"
	"euromni/internal/testdb"
	"euromni/internal/wasm"
)

func TestIRGeneratedWasmRunsAsInstalledModule(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)

	source := []byte(`<?php
add_action('woocommerce_product_query', function ($query) {
    return $query;
});
`)
	analysis, err := AnalyzePHPSource(string(source))
	if err != nil {
		t.Fatalf("AnalyzePHPSource() returned error: %v", err)
	}
	archive := buildZIP(t, map[string]string{
		"manifest.json":  `{"name":"demo.catalog-reader","version":"test-1","platform":"woocommerce","shop":"atelier-demo"}`,
		"src/plugin.php": string(source),
	})
	provenance, err := NewPackageProvenance(source, archive, SourceProvenance{
		SourceReference: "integration-test",
		ModuleVersion:   "test-1",
		PlatformVersion: "unknown",
		Adapter:         "woocommerce",
		AdapterVersion:  "1.0.0",
	})
	if err != nil {
		t.Fatalf("NewSourceProvenance() returned error: %v", err)
	}
	ir, err := LowerPHPAnalysisToIR("demo.catalog-reader", analysis, provenance)
	if err != nil {
		t.Fatalf("LowerPHPAnalysisToIR() returned error: %v", err)
	}
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("CompileIRToWasm() returned error: %v", err)
	}
	artifactRoot := filepath.Join(t.TempDir(), "artifacts")
	artifactStore, err := artifact.NewFileStore(artifactRoot)
	if err != nil {
		t.Fatalf("create artifact store: %v", err)
	}
	storedArtifact, manifest, err := PublishModuleBuild(ctx, artifactStore, moduleStore, seed.ModuleID, ir, wasmBytes)
	if err != nil {
		t.Fatalf("publish generated module build: %v", err)
	}
	storedWasm, err := artifactStore.Get(ctx, storedArtifact.SHA256)
	if err != nil {
		t.Fatalf("read published Wasm artifact: %v", err)
	}
	var registeredHash []byte
	var registeredManifest []byte
	if err := database.QueryRowContext(ctx, `SELECT wasm_sha256, build_manifest FROM modules WHERE id::text = $1`, seed.ModuleID).Scan(&registeredHash, &registeredManifest); err != nil {
		t.Fatalf("read registered build metadata: %v", err)
	}
	var storedManifest BuildManifest
	if err := json.Unmarshal(registeredManifest, &storedManifest); err != nil {
		t.Fatalf("decode registered build manifest: %v", err)
	}
	if !bytes.Equal(registeredHash, mustHexDigest(t, manifest.WASMSHA256)) {
		t.Fatalf("registered build metadata does not match published artifact")
	}
	if err := storedManifest.Validate(ir, storedWasm); err != nil {
		t.Fatalf("registered build manifest does not match IR/artifact: %v", err)
	}

	memory := store.NewMemory()
	products, err := memory.ListProducts(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("load expected catalog: %v", err)
	}
	expectedResponse := abi.EncodeOK("catalog.product.list", "", products)
	runtime := wasm.NewRuntime(&wasm.Host{Products: memory, Categories: memory})
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, storedWasm); err != nil {
		t.Fatalf("load generated Wasm as installed module: %v", err)
	}
	results, err := runtime.Run(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run")
	if err != nil {
		t.Fatalf("run generated Wasm: %v", err)
	}
	if len(results) != 1 || results[0] != int32(len(expectedResponse)) {
		t.Fatalf("guest result = %v, want successful catalog response length %d", results, len(expectedResponse))
	}

	dynamicIR := ModuleIR{
		Version:    IRVersion,
		Platform:   "woocommerce",
		Module:     "demo.dynamic-catalog-reader",
		Provenance: provenance,
		Triggers: []IRTrigger{{
			Kind:   "hook",
			Name:   "woocommerce_product_query",
			Fields: map[string]string{"product_id": "uuid"},
		}},
		Operations: []IROperation{{
			Call: "catalog.product.get",
			Input: map[string]IRValue{
				"product_id": {TriggerField: "product_id"},
			},
		}},
	}
	triggerPayload := []byte(`{"product_id":"` + seed.SecondProductID + `"}`)
	callEnvelope, err := BuildIRCallEnvelope(dynamicIR, triggerPayload)
	if err != nil {
		t.Fatalf("BuildIRCallEnvelope() returned error: %v", err)
	}
	dynamicWasm, err := CompileIRToWasm(dynamicIR)
	if err != nil {
		t.Fatalf("CompileIRToWasm() returned error for dynamic IR: %v", err)
	}
	dynamicArtifact, _, err := PublishModuleBuild(ctx, artifactStore, moduleStore, seed.ModuleID, dynamicIR, dynamicWasm)
	if err != nil {
		t.Fatalf("publish dynamic module build: %v", err)
	}
	storedDynamicWasm, err := artifactStore.Get(ctx, dynamicArtifact.SHA256)
	if err != nil {
		t.Fatalf("read published dynamic Wasm: %v", err)
	}
	if err := runtime.ReloadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, storedDynamicWasm); err != nil {
		t.Fatalf("reload dynamic installed Wasm: %v", err)
	}
	product, err := memory.GetProduct(ctx, seed.ShopID, seed.SecondProductID)
	if err != nil {
		t.Fatalf("load expected dynamic product: %v", err)
	}
	expectedDynamicResponse := abi.EncodeOK("catalog.product.get", "", product)
	results, err = runtime.RunWithInput(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", callEnvelope)
	if err != nil {
		t.Fatalf("run dynamic Wasm with trigger payload: %v", err)
	}
	if len(results) != 1 || results[0] != int32(len(expectedDynamicResponse)) {
		t.Fatalf("dynamic guest result = %v, want catalog response length %d", results, len(expectedDynamicResponse))
	}

	orphan, err := artifactStore.Put(ctx, bytes.NewReader([]byte("old unreferenced Wasm")))
	if err != nil {
		t.Fatalf("store orphan artifact: %v", err)
	}
	orphanPath := filepath.Join(artifactRoot, orphan.SHA256[:2], orphan.SHA256)
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(orphanPath, oldTime, oldTime); err != nil {
		t.Fatalf("age orphan artifact: %v", err)
	}
	removed, err := ReconcileModuleArtifactsWithGrace(ctx, artifactStore, moduleStore, 24*time.Hour)
	if err != nil {
		t.Fatalf("reconcile unreferenced artifacts: %v", err)
	}
	if len(removed) != 1 || removed[0].SHA256 != orphan.SHA256 {
		t.Fatalf("removed artifacts = %#v, want only orphan %s", removed, orphan.SHA256)
	}
	if _, err := artifactStore.Get(ctx, dynamicArtifact.SHA256); err != nil {
		t.Fatalf("reconciliation removed the installed Wasm: %v", err)
	}
	if _, err := artifactStore.Get(ctx, orphan.SHA256); !os.IsNotExist(err) {
		t.Fatalf("orphan read error = %v, want file not found", err)
	}
}

func TestRealPrestaShopNewProductsWasmRunsAsInstalledModule(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)

	fixturePaths, err := filepath.Glob("../../pipeline/fixtures/prestashop-ps_newproducts/ps_newproducts-*/ps_newproducts.php")
	if err != nil {
		t.Fatalf("find ps_newproducts fixture: %v", err)
	}
	if len(fixturePaths) != 1 {
		t.Fatalf("found %d ps_newproducts entrypoints, want exactly one; run pipeline/fetch_real_module_fixtures.sh", len(fixturePaths))
	}
	source, err := os.ReadFile(fixturePaths[0])
	if err != nil {
		t.Fatalf("read ps_newproducts fixture: %v", err)
	}
	analysis, err := AnalyzePHPSource(string(source))
	if err != nil {
		t.Fatalf("analyze ps_newproducts source: %v", err)
	}
	archive := buildZIP(t, map[string]string{
		"euromni-manifest.json":     `{"name":"ps_newproducts","platform":"prestashop"}`,
		"module/ps_newproducts.php": string(source),
	})
	provenance, err := NewPackageProvenance(source, archive, SourceProvenance{
		SourceReference: "prestashop/ps_newproducts@72e9476406319cbb91ac383121ca9967dc61f277",
		ModuleVersion:   "2.0.0",
		PlatformVersion: "9.x",
		Adapter:         "prestashop",
		AdapterVersion:  "1.0.0",
	})
	if err != nil {
		t.Fatalf("create source provenance: %v", err)
	}
	ir, err := LowerPHPAnalysisToIR("ps_newproducts", analysis, provenance)
	if err != nil {
		t.Fatalf("lower ps_newproducts source to IR: %v", err)
	}
	callEnvelope, err := BuildIRCallEnvelope(ir, []byte(`{"days":30,"limit":1}`))
	if err != nil {
		t.Fatalf("build displayHome call envelope: %v", err)
	}

	oldCreatedAt := time.Now().UTC().Add(-90 * 24 * time.Hour)
	if _, err := database.ExecContext(ctx, `UPDATE products SET created_at = $1`, oldCreatedAt); err != nil {
		t.Fatalf("age seeded catalog products: %v", err)
	}
	newestProductID := seed.SecondProductID
	for productID, age := range map[string]time.Duration{
		seed.ProductID:       4 * 24 * time.Hour,
		seed.SecondProductID: 2 * 24 * time.Hour,
		seed.ThirdProductID:  60 * 24 * time.Hour,
	} {
		if _, err := database.ExecContext(ctx, `UPDATE products SET created_at = $1 WHERE id::text = $2`, time.Now().UTC().Add(-age), productID); err != nil {
			t.Fatalf("set synthetic product age for %s: %v", productID, err)
		}
	}
	expectedProducts, err := moduleStore.ListNewProducts(ctx, seed.ShopID, 30, 1)
	if err != nil {
		t.Fatalf("list synthetic new products: %v", err)
	}
	if len(expectedProducts) != 1 || expectedProducts[0].ID != newestProductID {
		t.Fatalf("new product results = %#v, want only newest product %s", expectedProducts, newestProductID)
	}

	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("compile PrestaShop IR to Wasm: %v", err)
	}
	assertOnlyEUROMNICallImport(t, wasmBytes)
	artifactStore, err := artifact.NewFileStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("create artifact store: %v", err)
	}
	storedArtifact, manifest, err := PublishModuleBuild(ctx, artifactStore, moduleStore, seed.ModuleID, ir, wasmBytes)
	if err != nil {
		t.Fatalf("publish PrestaShop Wasm build: %v", err)
	}
	storedWasm, err := artifactStore.Get(ctx, storedArtifact.SHA256)
	if err != nil {
		t.Fatalf("read published PrestaShop Wasm: %v", err)
	}
	if err := manifest.Validate(ir, storedWasm); err != nil {
		t.Fatalf("validate published build manifest: %v", err)
	}

	runtime := wasm.NewRuntime(&wasm.Host{Products: moduleStore, Categories: moduleStore})
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, storedWasm); err != nil {
		t.Fatalf("load published PrestaShop module: %v", err)
	}
	results, err := runtime.RunWithInput(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", callEnvelope)
	if err != nil {
		t.Fatalf("execute published PrestaShop module: %v", err)
	}
	expectedResponse := abi.EncodeOK("catalog.product.newest", "", expectedProducts)
	if len(results) != 1 || results[0] != int32(len(expectedResponse)) {
		t.Fatalf("guest result = %v, want successful new-products response length %d", results, len(expectedResponse))
	}
}

func mustHexDigest(t *testing.T, value string) []byte {
	t.Helper()
	digest, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode SHA-256 %q: %v", value, err)
	}
	return digest
}
