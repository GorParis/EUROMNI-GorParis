//go:build integration

package pipeline

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
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
	modules, err := widgetRunner.ListModules(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("list registered modules: %v", err)
	}
	var registered *store.ModuleInventoryEntry
	for i := range modules {
		if modules[i].Name == store.NewProductsModuleName {
			registered = &modules[i]
			break
		}
	}
	if registered == nil || registered.Platform != "prestashop" || registered.ModuleVersion != "2.0.1" ||
		registered.PlatformVersion != "euromni-test-adapter" || registered.Adapter != "prestashop" || registered.Status != "installed" ||
		!registered.CanInstall || !registered.CanUninstall || registered.WASMSHA256 != manifest.WASMSHA256 {
		t.Fatalf("module inventory entry = %+v, want the installed PrestaShop build metadata", registered)
	}
	if len(registered.Builds) != 1 || !registered.Builds[0].Current ||
		!registered.Builds[0].Installed || !registered.Builds[0].CanPromote {
		t.Fatalf("initial build inventory = %+v, want current installed promotable build", registered.Builds)
	}
	secondWasm := append([]byte(nil), wasmBytes...)
	secondWasm = append(secondWasm, 0x00, 0x10, 0x0e)
	secondWasm = append(secondWasm, []byte("build-revision")...)
	secondWasm = append(secondWasm, 0x01)
	secondArtifact, secondManifest, err := PublishModuleBuild(
		ctx, artifactStore, moduleStore, moduleID, ir, secondWasm,
	)
	if err != nil {
		t.Fatalf("publish second immutable module build: %v", err)
	}
	if secondArtifact.SHA256 == manifest.WASMSHA256 {
		t.Fatal("second module build has the same Wasm digest")
	}
	if installed, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID); err != nil ||
		installed == nil || hex.EncodeToString(installed.WASMSHA256) != manifest.WASMSHA256 {
		t.Fatalf("publishing second build changed installed pin: module=%+v error=%v", installed, err)
	}
	modules, err = widgetRunner.ListModules(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("list builds before promotion: %v", err)
	}
	registered = nil
	for i := range modules {
		if modules[i].Name == store.NewProductsModuleName {
			registered = &modules[i]
			break
		}
	}
	if registered == nil || len(registered.Builds) != 2 {
		t.Fatalf("module inventory after second publication = %+v, want two available builds", registered)
	}
	var secondBuildListed bool
	for _, build := range registered.Builds {
		if build.WASMSHA256 == secondManifest.WASMSHA256 {
			secondBuildListed = build.Current && !build.Installed && build.CanPromote
		}
	}
	if !secondBuildListed {
		t.Fatalf("second build inventory = %+v, want current, promotable, and not installed", registered.Builds)
	}
	unavailableRunner, err := wasm.NewNewProductsWidgetRunner(moduleStore, unavailableModuleArtifacts{}, widgetRuntime)
	if err != nil {
		t.Fatalf("configure module runner with unavailable artifact store: %v", err)
	}
	if err := unavailableRunner.PromoteModule(ctx, seed.ShopID, store.NewProductsModuleName, secondManifest.WASMSHA256); err == nil {
		t.Fatal("promotion succeeded without a verifiable Wasm artifact")
	}
	stillPinned, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID)
	if err != nil || hex.EncodeToString(stillPinned.WASMSHA256) != manifest.WASMSHA256 {
		t.Fatalf("failed artifact verification changed installed build: module=%+v error=%v", stillPinned, err)
	}
	if err := widgetRunner.PromoteModule(ctx, seed.ShopID, store.NewProductsModuleName, secondManifest.WASMSHA256); err != nil {
		t.Fatalf("promote second build through module manager: %v", err)
	}
	promotedModule, err := moduleStore.GetInstalledModule(ctx, seed.ShopID, moduleID)
	if err != nil || hex.EncodeToString(promotedModule.WASMSHA256) != secondManifest.WASMSHA256 {
		t.Fatalf("installed build after promotion = %+v, error = %v", promotedModule, err)
	}
	promotedProducts, promotedInstalled, err := widgetRunner.RunNewProducts(ctx, seed.ShopID)
	if err != nil || !promotedInstalled || len(promotedProducts) != len(expectedProducts) {
		t.Fatalf("widget after build promotion = (%d products, installed=%t, error=%v)",
			len(promotedProducts), promotedInstalled, err)
	}
	if err := widgetRunner.UninstallModule(ctx, seed.ShopID, store.NewProductsModuleName); err != nil {
		t.Fatalf("reversibly uninstall module: %v", err)
	}
	if products, installed, err := widgetRunner.RunNewProducts(ctx, seed.ShopID); err != nil || installed || len(products) != 0 {
		t.Fatalf("widget after uninstall = (%v, installed=%t, error=%v), want empty and disabled", products, installed, err)
	}
	unavailableRunner, err = wasm.NewNewProductsWidgetRunner(moduleStore, unavailableModuleArtifacts{}, widgetRuntime)
	if err != nil {
		t.Fatalf("configure module runner with unavailable artifact store: %v", err)
	}
	if err := unavailableRunner.InstallModule(ctx, seed.ShopID, store.NewProductsModuleName); err == nil {
		t.Fatal("installation succeeded without a verifiable Wasm artifact")
	}
	modules, err = widgetRunner.ListModules(ctx, seed.ShopID)
	if err != nil {
		t.Fatalf("list modules after failed artifact verification: %v", err)
	}
	foundDisabled := false
	for _, module := range modules {
		if module.Name == store.NewProductsModuleName {
			foundDisabled = module.Status == "disabled"
		}
	}
	if !foundDisabled {
		t.Fatalf("module inventory after failed artifact verification = %+v, want ps_newproducts disabled", modules)
	}
	if err := widgetRunner.InstallModule(ctx, seed.ShopID, store.NewProductsModuleName); err != nil {
		t.Fatalf("reinstall verified module build: %v", err)
	}
	reinstalledSettings, err = moduleStore.GetNewProductsSettings(ctx, seed.ShopID)
	if err != nil || reinstalledSettings != settings {
		t.Fatalf("settings after reinstallation = %+v, error = %v; want preserved %+v", reinstalledSettings, err, settings)
	}
	if products, installed, err := widgetRunner.RunNewProducts(ctx, seed.ShopID); err != nil || !installed || len(products) != len(expectedProducts) {
		t.Fatalf("widget after reinstallation = (%d products, installed=%t, error=%v), want %d products",
			len(products), installed, err, len(expectedProducts))
	}
}

type unavailableModuleArtifacts struct{}

func (unavailableModuleArtifacts) Put(context.Context, io.Reader) (artifact.Artifact, error) {
	return artifact.Artifact{}, errors.New("artifact writes are unavailable")
}

func (unavailableModuleArtifacts) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("artifact is unavailable")
}
