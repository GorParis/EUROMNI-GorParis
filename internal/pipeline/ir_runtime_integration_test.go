//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
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

func TestRejectedBuildPublicationPreservesCurrentBuildAndInstalledPin(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)
	moduleID, err := store.NewCatalogID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO modules (id, name) VALUES ($1, $2)
	`, moduleID, "demo.catalog-reader"); err != nil {
		t.Fatalf("create test module: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO shop_modules (shop_id, module_id, status, capabilities)
		VALUES ($1, $2, 'installed', ARRAY['catalog']::text[])
	`, seed.ShopID, moduleID); err != nil {
		t.Fatalf("create installed test module: %v", err)
	}

	ir := testCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("compile valid module build: %v", err)
	}
	artifactStore, err := artifact.NewFileStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("create isolated artifact store: %v", err)
	}
	published, _, err := PublishModuleBuild(ctx, artifactStore, moduleStore, moduleID, ir, wasmBytes)
	if err != nil {
		t.Fatalf("publish valid baseline build: %v", err)
	}
	installedBefore, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID)
	if err != nil {
		t.Fatalf("read baseline installed module: %v", err)
	}
	if installedBefore.WASMSHA256 == nil || hex.EncodeToString(installedBefore.WASMSHA256) != published.SHA256 {
		t.Fatalf("baseline install pin = %x, want %s", installedBefore.WASMSHA256, published.SHA256)
	}

	tamperedWasm := append([]byte(nil), wasmBytes...)
	tamperedWasm[len(tamperedWasm)-1] ^= 1
	tamperedDigest := sha256.Sum256(tamperedWasm)
	if _, _, err := PublishModuleBuild(ctx, artifactStore, moduleStore, moduleID, ir, tamperedWasm); err == nil {
		t.Fatal("publication accepted Wasm with a modified compiler provenance section")
	}
	if _, err := artifactStore.Get(ctx, hex.EncodeToString(tamperedDigest[:])); !os.IsNotExist(err) {
		t.Fatalf("tampered artifact read error = %v, want not found", err)
	}

	var currentHash []byte
	if err := database.QueryRowContext(ctx, `
		SELECT wasm_sha256 FROM modules WHERE id = $1
	`, moduleID).Scan(&currentHash); err != nil {
		t.Fatalf("read current build after rejected publication: %v", err)
	}
	if hex.EncodeToString(currentHash) != published.SHA256 {
		t.Fatalf("current build hash = %x, want unchanged %s", currentHash, published.SHA256)
	}
	installedAfter, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID)
	if err != nil {
		t.Fatalf("read installed module after rejected publication: %v", err)
	}
	if !bytes.Equal(installedAfter.WASMSHA256, installedBefore.WASMSHA256) {
		t.Fatalf("rejected publication changed installation pin from %x to %x", installedBefore.WASMSHA256, installedAfter.WASMSHA256)
	}
}

func TestBuildRegistrationFailurePreservesCurrentBuildAndInstalledPin(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)
	moduleID, err := store.NewCatalogID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO modules (id, name) VALUES ($1, $2)
	`, moduleID, "demo.registration-failure"); err != nil {
		t.Fatalf("create test module: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO shop_modules (shop_id, module_id, status, capabilities)
		VALUES ($1, $2, 'installed', ARRAY['catalog']::text[])
	`, seed.ShopID, moduleID); err != nil {
		t.Fatalf("create installed test module: %v", err)
	}

	ir := testCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("compile baseline module build: %v", err)
	}
	artifactStore, err := artifact.NewFileStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("create isolated artifact store: %v", err)
	}
	published, _, err := PublishModuleBuild(ctx, artifactStore, moduleStore, moduleID, ir, wasmBytes)
	if err != nil {
		t.Fatalf("publish baseline build: %v", err)
	}
	installedBefore, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID)
	if err != nil {
		t.Fatalf("read baseline installed module: %v", err)
	}

	if _, err := database.ExecContext(ctx, `
		CREATE FUNCTION reject_test_build_update() RETURNS trigger AS $$
		BEGIN
			IF OLD.name = 'demo.registration-failure'
				AND NEW.wasm_sha256 IS DISTINCT FROM OLD.wasm_sha256 THEN
				RAISE EXCEPTION 'injected build registration failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_test_build_update
		BEFORE UPDATE ON modules
		FOR EACH ROW EXECUTE FUNCTION reject_test_build_update();
	`); err != nil {
		t.Fatalf("install temporary database failure trigger: %v", err)
	}

	failedIR := testCatalogIR()
	failedIR.Provenance.SourceReference = "database-failure-injected"
	failedWASM, err := CompileIRToWasm(failedIR)
	if err != nil {
		t.Fatalf("compile candidate for database failure: %v", err)
	}
	failedDigest := sha256.Sum256(failedWASM)
	if _, _, err := PublishModuleBuild(ctx, artifactStore, moduleStore, moduleID, failedIR, failedWASM); err == nil {
		t.Fatal("publication succeeded despite injected database registration failure")
	}
	storedFailedWASM, err := artifactStore.Get(ctx, hex.EncodeToString(failedDigest[:]))
	if err != nil || !bytes.Equal(storedFailedWASM, failedWASM) {
		t.Fatalf("failed registration artifact read size/error = %d/%v, want immutable stored orphan", len(storedFailedWASM), err)
	}

	var currentHash []byte
	if err := database.QueryRowContext(ctx, `
		SELECT wasm_sha256 FROM modules WHERE id = $1
	`, moduleID).Scan(&currentHash); err != nil {
		t.Fatalf("read current build after registration failure: %v", err)
	}
	if hex.EncodeToString(currentHash) != published.SHA256 {
		t.Fatalf("current build hash = %x, want unchanged %s", currentHash, published.SHA256)
	}
	installedAfter, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID)
	if err != nil {
		t.Fatalf("read installed module after registration failure: %v", err)
	}
	if !bytes.Equal(installedAfter.WASMSHA256, installedBefore.WASMSHA256) {
		t.Fatalf("registration failure changed installation pin from %x to %x", installedBefore.WASMSHA256, installedAfter.WASMSHA256)
	}
	referenced, err := moduleStore.ListModuleWASMDigests(ctx)
	if err != nil {
		t.Fatalf("list module build references after registration failure: %v", err)
	}
	for _, digest := range referenced {
		if digest == hex.EncodeToString(failedDigest[:]) {
			t.Fatal("failed build registration added its artifact to the module registry")
		}
	}
}

func TestTransportedSignedBuildPackageRunsAsInstalledModule(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)
	manifestJSON, irJSON, wasmBytes := validBuildPackageJSON(t)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test package signing key: %v", err)
	}
	signature, err := SignBuildPackage(manifestJSON, irJSON, wasmBytes, privateKey)
	if err != nil {
		t.Fatalf("sign valid build package: %v", err)
	}
	signatureJSON, err := MarshalBuildPackageSignature(signature)
	if err != nil {
		t.Fatalf("encode build package signature: %v", err)
	}
	transport := buildPackageZIP(t, map[string][]byte{
		"signature.json": signatureJSON,
		"manifest.json":  manifestJSON,
		"module.ir.json": irJSON,
		"module.wasm":    wasmBytes,
	})
	received, err := ReadSignedBuildPackageArchive(transport)
	if err != nil {
		t.Fatalf("read transported signed package: %v", err)
	}
	validatedManifest, err := ValidateSignedBuildPackage(
		received.SignatureJSON, received.ManifestJSON, received.IRJSON, received.WASMBytes,
		map[string]ed25519.PublicKey{signature.KeyID: publicKey},
	)
	if err != nil {
		t.Fatalf("validate transported signed package: %v", err)
	}
	var ir ModuleIR
	if err := decodeStrictJSON(received.IRJSON, &ir); err != nil {
		t.Fatalf("decode validated package IR: %v", err)
	}

	moduleID, err := store.NewCatalogID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO modules (id, name) VALUES ($1, $2)
	`, moduleID, ir.Module); err != nil {
		t.Fatalf("create test module: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO shop_modules (shop_id, module_id, status, capabilities)
		VALUES ($1, $2, 'installed', ARRAY['catalog']::text[])
	`, seed.ShopID, moduleID); err != nil {
		t.Fatalf("create installed test module: %v", err)
	}
	artifactStore, err := artifact.NewFileStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("create isolated artifact store: %v", err)
	}
	transportedArtifact, publishedManifest, err := PublishModuleBuild(
		ctx, artifactStore, moduleStore, moduleID, ir, received.WASMBytes,
	)
	if err != nil {
		t.Fatalf("publish validated transported package: %v", err)
	}
	if publishedManifest.WASMSHA256 != validatedManifest.WASMSHA256 ||
		transportedArtifact.SHA256 != validatedManifest.WASMSHA256 {
		t.Fatalf("published digest = %q/%q, want validated package digest %q",
			publishedManifest.WASMSHA256, transportedArtifact.SHA256, validatedManifest.WASMSHA256)
	}
	storedWasm, err := artifactStore.Get(ctx, transportedArtifact.SHA256)
	if err != nil {
		t.Fatalf("read transported Wasm from artifact store: %v", err)
	}
	if !bytes.Equal(storedWasm, received.WASMBytes) {
		t.Fatal("stored Wasm differs from the validated transported package")
	}

	memory := store.NewMemory()
	product, err := memory.GetProduct(ctx, seed.ShopID, seed.ProductID)
	if err != nil {
		t.Fatalf("load expected catalog product: %v", err)
	}
	expectedResponse := abi.EncodeOK("catalog.product.get", "", product)
	runtime := wasm.NewRuntime(&wasm.Host{Products: memory, Categories: memory})
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, moduleID, storedWasm); err != nil {
		t.Fatalf("load transported Wasm as installed module: %v", err)
	}
	results, err := runtime.Run(ctx, moduleStore, seed.ShopID, moduleID, "run")
	if err != nil {
		t.Fatalf("run transported Wasm: %v", err)
	}
	if len(results) != 1 || results[0] != int32(len(expectedResponse)) {
		t.Fatalf("guest result = %v, want successful catalog response length %d", results, len(expectedResponse))
	}
}

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
	if err := moduleStore.PromoteInstalledModuleBuild(ctx, seed.ShopID, seed.ModuleID, dynamicArtifact.SHA256); err != nil {
		t.Fatalf("promote dynamic build for the test shop: %v", err)
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
	if _, err := database.ExecContext(ctx, `
		UPDATE shop_modules
		SET config = $3::jsonb
		WHERE shop_id::text = $1 AND module_id::text = $2
	`, seed.ShopID, seed.ModuleID, `{"days":30,"limit":1}`); err != nil {
		t.Fatalf("configure installed PrestaShop module: %v", err)
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
	responseBytes, err := runtime.RunInstalledWithInputAndResponse(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", nil)
	if err != nil {
		t.Fatalf("execute published PrestaShop module: %v", err)
	}
	var response abi.Response
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("decode published PrestaShop response: %v", err)
	}
	if !response.OK || response.Call != "catalog.product.newest" {
		t.Fatalf("published PrestaShop response = %s, want successful new-products call", responseBytes)
	}
	responseData, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatalf("encode published PrestaShop product data: %v", err)
	}
	var actualProducts []store.ProductView
	if err := json.Unmarshal(responseData, &actualProducts); err != nil {
		t.Fatalf("decode published PrestaShop products: %v", err)
	}
	expectedData, err := json.Marshal(expectedProducts)
	if err != nil {
		t.Fatalf("encode expected PrestaShop product data: %v", err)
	}
	actualData, err := json.Marshal(actualProducts)
	if err != nil {
		t.Fatalf("encode actual PrestaShop product data: %v", err)
	}
	if !bytes.Equal(actualData, expectedData) {
		t.Fatalf("published PrestaShop products = %s, want %s", actualData, expectedData)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE shop_modules
		SET config = $3::jsonb
		WHERE shop_id::text = $1 AND module_id::text = $2
	`, seed.ShopID, seed.ModuleID, `{"days":0,"limit":8}`); err != nil {
		t.Fatalf("disable installed PrestaShop module widget: %v", err)
	}
	responseBytes, err = runtime.RunInstalledWithInputAndResponse(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", nil)
	if err != nil {
		t.Fatalf("execute module after changing installed settings: %v", err)
	}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("decode disabled PrestaShop response: %v", err)
	}
	if !response.OK || response.Call != "catalog.product.newest" {
		t.Fatalf("disabled PrestaShop response = %s, want successful new-products call", responseBytes)
	}
	responseData, err = json.Marshal(response.Data)
	if err != nil {
		t.Fatalf("encode disabled PrestaShop product data: %v", err)
	}
	if string(responseData) != "[]" {
		t.Fatalf("disabled PrestaShop products = %s, want []", responseData)
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
