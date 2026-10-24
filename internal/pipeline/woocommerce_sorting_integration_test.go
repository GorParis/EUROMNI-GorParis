//go:build integration

package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"euromni/internal/abi"
	"euromni/internal/artifact"
	"euromni/internal/seed"
	"euromni/internal/store"
	"euromni/internal/testdb"
	"euromni/internal/wasm"
)

func TestRealWooCommerceSortingArchiveRunsAsPublishedWasm(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)

	archive, err := os.ReadFile("../../pipeline/fixtures/woocommerce-extra-product-sorting-options.zip")
	if err != nil {
		t.Fatalf("read pinned WooCommerce archive: %v", err)
	}
	ir, wasmBytes, err := BuildWooCommerceSortingArchive(archive)
	if err != nil {
		t.Fatalf("build WooCommerce sorting module: %v", err)
	}
	validated, err := ValidateIR(ir)
	if err != nil {
		t.Fatalf("validate WooCommerce IR: %v", err)
	}
	if !reflect.DeepEqual(validated.Capabilities, []string{"catalog"}) {
		t.Fatalf("capabilities = %#v, want only catalog", validated.Capabilities)
	}

	artifactStore, err := artifact.NewFileStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("create private artifact store: %v", err)
	}
	published, manifest, err := PublishModuleBuild(ctx, artifactStore, moduleStore, seed.ModuleID, ir, wasmBytes)
	if err != nil {
		t.Fatalf("publish WooCommerce module build: %v", err)
	}
	storedWasm, err := artifactStore.Get(ctx, published.SHA256)
	if err != nil {
		t.Fatalf("read published Wasm artifact: %v", err)
	}
	if err := manifest.Validate(ir, storedWasm); err != nil {
		t.Fatalf("validate published build manifest: %v", err)
	}
	assertOnlyEUROMNICallImport(t, storedWasm)

	runtime := wasm.NewRuntime(&wasm.Host{Products: moduleStore, Categories: moduleStore})
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, storedWasm); err != nil {
		t.Fatalf("load published WooCommerce module: %v", err)
	}
	for _, test := range []struct {
		orderby string
		want    []string
	}{
		{orderby: "alphabetical", want: []string{"bougie-cedre", "cabas-lin", "ceinture-cuir", "chemise-lin", "tasse-gres"}},
		{orderby: "reverse_alpha", want: []string{"tasse-gres", "chemise-lin", "ceinture-cuir", "cabas-lin", "bougie-cedre"}},
	} {
		t.Run(test.orderby, func(t *testing.T) {
			trigger, err := json.Marshal(map[string]string{
				"locale":  "fr-FR",
				"orderby": test.orderby,
			})
			if err != nil {
				t.Fatal(err)
			}
			input, err := BuildIRCallEnvelope(ir, trigger)
			if err != nil {
				t.Fatalf("build hook call envelope: %v", err)
			}
			responseBytes, err := runtime.RunInstalledWithInputAndResponse(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", input)
			if err != nil {
				t.Fatalf("execute installed WooCommerce module: %v", err)
			}
			var response abi.Response
			if err := json.Unmarshal(responseBytes, &response); err != nil {
				t.Fatalf("decode ABI response: %v", err)
			}
			if !response.OK {
				t.Fatalf("WooCommerce sorting call failed: %s", responseBytes)
			}
			encoded, err := json.Marshal(response.Data)
			if err != nil {
				t.Fatal(err)
			}
			var products []store.ProductView
			if err := json.Unmarshal(encoded, &products); err != nil {
				t.Fatalf("decode sorted products: %v", err)
			}
			got := make([]string, len(products))
			for i, product := range products {
				got[i] = product.Slug
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("WooCommerce sort result = %#v, want oracle %#v", got, test.want)
			}
		})
	}

	stockRuntime := wasm.NewRuntime(&wasm.Host{
		Products: wooStockProducts{
			{
				ID: "product-c", Slug: "citron", Translations: []store.TranslationView{{Locale: "fr-FR", Name: "Citron"}},
				Variants: []store.VariantView{{Inventory: store.InventoryView{Quantity: 9, Track: true}}},
			},
			{
				ID: "product-b", Slug: "banane", Translations: []store.TranslationView{{Locale: "fr-FR", Name: "Banane"}},
				Variants: []store.VariantView{{Inventory: store.InventoryView{Quantity: 3, Track: true}}},
			},
			{
				ID: "product-a", Slug: "abricot", Translations: []store.TranslationView{{Locale: "fr-FR", Name: "Abricot"}},
				Variants: []store.VariantView{{Inventory: store.InventoryView{Quantity: 9, Track: true}}},
			},
		},
		Categories: moduleStore,
	})
	t.Cleanup(stockRuntime.Close)
	if err := stockRuntime.LoadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, storedWasm); err != nil {
		t.Fatalf("load published WooCommerce module for stock sort: %v", err)
	}
	trigger, err := json.Marshal(map[string]string{"locale": "fr-FR", "orderby": "by_stock"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := BuildIRCallEnvelope(ir, trigger)
	if err != nil {
		t.Fatalf("build stock-sort hook call envelope: %v", err)
	}
	responseBytes, err := stockRuntime.RunInstalledWithInputAndResponse(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", input)
	if err != nil {
		t.Fatalf("execute installed WooCommerce stock sort: %v", err)
	}
	var response abi.Response
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("decode stock-sort ABI response: %v", err)
	}
	if !response.OK {
		t.Fatalf("WooCommerce stock sorting call failed: %s", responseBytes)
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatal(err)
	}
	var products []store.ProductView
	if err := json.Unmarshal(encoded, &products); err != nil {
		t.Fatalf("decode stock-sorted products: %v", err)
	}
	got := make([]string, len(products))
	for i, product := range products {
		got[i] = product.Slug
	}
	want := []string{"abricot", "citron", "banane"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("WooCommerce stock sort result = %#v, want oracle %#v", got, want)
	}
}

type wooStockProducts []store.ProductView

func (wooStockProducts) GetProduct(context.Context, string, string) (*store.ProductView, error) {
	return nil, store.ErrNotFound
}

func (p wooStockProducts) ListProducts(context.Context, string) ([]store.ProductView, error) {
	return append([]store.ProductView(nil), p...), nil
}
