//go:build integration

package pipeline

import (
	"context"
	"errors"
	"os"
	"testing"

	"euromni/internal/abi"
	"euromni/internal/artifact"
	"euromni/internal/seed"
	"euromni/internal/store"
	"euromni/internal/testdb"
	"euromni/internal/wasm"
)

func TestUpstreamPrestaShopArchivePublishesAndRunsInIsolatedCMS(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)
	archive, err := os.ReadFile("../../pipeline/fixtures/prestashop-ps_newproducts.zip")
	if err != nil {
		t.Fatalf("read upstream archive: %v", err)
	}
	ir, wasmBytes, err := BuildPrestaShopNewProductsArchive(archive, "euromni-test-adapter")
	if err != nil {
		t.Fatalf("build upstream module: %v", err)
	}
	settings := store.NewProductsSettings{Days: 3650, Limit: 8}
	artifactStore, err := artifact.NewFileStore(t.TempDir() + "/artifacts")
	if err != nil {
		t.Fatalf("create private artifact store: %v", err)
	}
	published, manifest, moduleID, err := PublishNewProductsModuleBuild(ctx, artifactStore, moduleStore, seed.ShopID, settings, ir, wasmBytes)
	if err != nil {
		t.Fatalf("publish and install upstream module: %v", err)
	}
	storedWasm, err := artifactStore.Get(ctx, published.SHA256)
	if err != nil {
		t.Fatalf("read published Wasm: %v", err)
	}
	if err := manifest.Validate(ir, storedWasm); err != nil {
		t.Fatalf("verify published build manifest: %v", err)
	}
	installed, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID)
	installedSettings, settingsErr := moduleStore.GetNewProductsSettings(ctx, seed.ShopID)
	if err != nil || settingsErr != nil || installed.Name != store.NewProductsModuleName || installedSettings != settings {
		t.Fatalf("installed module = %+v, error = %v", installed, err)
	}
	if _, _, _, err := PublishNewProductsModuleBuild(ctx, artifactStore, moduleStore, seed.ShopID,
		store.NewProductsSettings{Days: 0, Limit: 1}, ir, wasmBytes); err != nil {
		t.Fatalf("repeat installation: %v", err)
	}
	reinstalledSettings, err := moduleStore.GetNewProductsSettings(ctx, seed.ShopID)
	if err != nil || reinstalledSettings != settings {
		t.Fatalf("reinstall changed settings to %+v, error = %v", reinstalledSettings, err)
	}
	if _, err := moduleStore.GetInstalledModule(ctx, seed.OtherShop, moduleID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("module installed into another shop: error = %v", err)
	}
	expectedProducts, err := moduleStore.ListNewProducts(ctx, seed.ShopID, settings.Days, settings.Limit)
	if err != nil {
		t.Fatalf("query expected new products: %v", err)
	}

	runtime := wasm.NewRuntime(&wasm.Host{Products: moduleStore, Categories: moduleStore})
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, moduleID, storedWasm); err != nil {
		t.Fatalf("load installed Wasm: %v", err)
	}
	results, err := runtime.Run(ctx, moduleStore, seed.ShopID, moduleID, "run")
	if err != nil {
		t.Fatalf("run installed Wasm: %v", err)
	}
	expected := abi.EncodeOK("catalog.product.newest", "", expectedProducts)
	if len(results) != 1 || results[0] != int32(len(expected)) {
		t.Fatalf("runtime result = %v, want ABI response size %d", results, len(expected))
	}

	widgetRuntime := wasm.NewRuntime(&wasm.Host{Products: moduleStore, Categories: moduleStore})
	t.Cleanup(widgetRuntime.Close)
	widgetRunner, err := wasm.NewNewProductsWidgetRunner(moduleStore, artifactStore, widgetRuntime)
	if err != nil {
		t.Fatalf("configure new-products widget runner: %v", err)
	}
	widgetProducts, moduleInstalled, err := widgetRunner.RunNewProducts(ctx, seed.ShopID)
	if err != nil || !moduleInstalled || len(widgetProducts) != len(expectedProducts) {
		t.Fatalf("new-products widget = (%+v, installed=%t, error=%v), want %d products",
			widgetProducts, moduleInstalled, err, len(expectedProducts))
	}
	for i, product := range expectedProducts {
		if widgetProducts[i].ID != product.ID || widgetProducts[i].Slug != product.Slug {
			t.Fatalf("new-products widget product %d = (%s, %s), want (%s, %s)",
				i, widgetProducts[i].ID, widgetProducts[i].Slug, product.ID, product.Slug)
		}
	}
}
