//go:build integration

package pipeline

import (
	"bytes"
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

func TestSelfnamedProductPreviewWasmRunsWithoutPersistence(t *testing.T) {
	ctx := context.Background()
	database := testdb.Start(t)
	moduleStore := store.NewSQL(database)
	source := []byte("Selfnamed 2.0.19 bounded catalog preview characterization")
	archive := buildZIP(t, map[string]string{
		"manifest.json": `{"name":"selfnamed-catalog-preview","version":"2.0.19","platform":"woocommerce"}`,
		"source.txt":    string(source),
	})
	provenance, err := NewPackageProvenance(source, archive, SourceProvenance{
		SourceReference: "selfnamed-catalog-preview-characterization",
		ModuleVersion:   "2.0.19",
		PlatformVersion: "10.9.0",
		Adapter:         "woocommerce",
		AdapterVersion:  "1.0.0",
	})
	if err != nil {
		t.Fatalf("create preview provenance: %v", err)
	}
	ir := ModuleIR{
		Version:    IRVersion,
		Platform:   "woocommerce",
		Module:     "selfnamed-catalog-preview",
		Provenance: provenance,
		Triggers: []IRTrigger{{
			Kind:   "manual",
			Name:   "supplier_product.preview",
			Fields: map[string]string{"product_json": "string"},
		}},
		Operations: []IROperation{{
			Call:  "catalog.supplier.product.preview",
			Input: map[string]IRValue{"product_json": {TriggerField: "product_json"}},
		}},
	}
	validated, err := ValidateIR(ir)
	if err != nil {
		t.Fatalf("validate preview IR: %v", err)
	}
	if len(validated.Capabilities) != 1 || validated.Capabilities[0] != string(wasm.CapCatalog) {
		t.Fatalf("preview capabilities = %v, want catalog only", validated.Capabilities)
	}
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("compile preview IR to Wasm: %v", err)
	}
	assertOnlyEUROMNICallImport(t, wasmBytes)

	artifactStore, err := artifact.NewFileStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatalf("create artifact store: %v", err)
	}
	storedArtifact, _, err := PublishModuleBuild(ctx, artifactStore, moduleStore, seed.ModuleID, ir, wasmBytes)
	if err != nil {
		t.Fatalf("publish preview Wasm build: %v", err)
	}
	storedWasm, err := artifactStore.Get(ctx, storedArtifact.SHA256)
	if err != nil {
		t.Fatalf("read published preview Wasm: %v", err)
	}

	fixtureBytes, err := os.ReadFile("../../pipeline/selfnamed-pilot/catalog-products.json")
	if err != nil {
		t.Fatalf("read synthetic Selfnamed catalog fixtures: %v", err)
	}
	fixtureDecoder := json.NewDecoder(bytes.NewReader(fixtureBytes))
	fixtureDecoder.DisallowUnknownFields()
	var fixtures []json.RawMessage
	if err := fixtureDecoder.Decode(&fixtures); err != nil {
		t.Fatalf("decode synthetic Selfnamed catalog fixtures: %v", err)
	}
	if len(fixtures) != 3 {
		t.Fatalf("Selfnamed fixture count = %d, want 3", len(fixtures))
	}
	expectedByID := map[int]map[string]any{
		842002: {
			"provider": "selfnamed", "external_id": "842002", "publication_status": "draft",
			"name": "Synthetic Catalog Balm", "description": "Synthetic description used to characterize product mapping.",
			"locale": nil, "price": map[string]any{"amount": "28.82", "currency": nil},
			"inventory": map[string]any{"status": "instock", "track_quantity": false},
			"weight":    map[string]any{"grams": float64(176), "unit": nil},
			"volume":    map[string]any{"value": float64(176), "unit": nil},
			"tags":      []any{}, "unresolved": []any{"currency", "locale", "volume_unit"},
		},
		842003: {
			"provider": "selfnamed", "external_id": "842003", "publication_status": "draft",
			"name": "Synthetic Out-of-Stock Balm", "description": "Synthetic unavailable stock case.",
			"locale": nil, "price": map[string]any{"amount": "17.4", "currency": nil},
			"inventory": map[string]any{"status": "outofstock", "track_quantity": false},
			"weight":    map[string]any{"grams": float64(1001), "unit": nil},
			"volume":    map[string]any{"value": float64(88), "unit": nil},
			"tags":      []any{"synthetic", "edge-case"}, "unresolved": []any{"currency", "locale", "volume_unit"},
		},
		842004: {
			"provider": "selfnamed", "external_id": "842004", "publication_status": "draft",
			"name": "Synthetic Decimal-Price Balm", "description": "",
			"locale": nil, "price": map[string]any{"amount": "0.1", "currency": nil},
			"inventory": map[string]any{"status": "instock", "track_quantity": false},
			"weight":    map[string]any{"grams": float64(1000), "unit": nil},
			"volume":    map[string]any{"value": float64(1), "unit": nil},
			"tags":      []any{}, "unresolved": []any{"currency", "locale", "volume_unit"},
		},
	}

	var productCountBefore int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM products WHERE shop_id = $1`, seed.ShopID).Scan(&productCountBefore); err != nil {
		t.Fatalf("count products before preview: %v", err)
	}
	host := &wasm.Host{}
	runtime := wasm.NewRuntime(host)
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, storedWasm); err != nil {
		t.Fatalf("load preview Wasm build: %v", err)
	}
	seen := make(map[int]struct{}, len(fixtures))
	for _, rawFixture := range fixtures {
		fixtureDecoder := json.NewDecoder(bytes.NewReader(rawFixture))
		fixtureDecoder.UseNumber()
		fixtureDecoder.DisallowUnknownFields()
		var fixture selfnamedCatalogFixture
		if err := fixtureDecoder.Decode(&fixture); err != nil {
			t.Fatalf("decode fixture metadata: %v", err)
		}
		if _, ok := seen[fixture.ID]; ok {
			t.Fatalf("duplicate Selfnamed fixture ID %d", fixture.ID)
		}
		seen[fixture.ID] = struct{}{}
		expectedData, ok := expectedByID[fixture.ID]
		if !ok {
			t.Fatalf("no Wasm preview oracle for Selfnamed fixture %d", fixture.ID)
		}

		trigger, err := json.Marshal(map[string]string{"product_json": string(rawFixture)})
		if err != nil {
			t.Fatalf("encode fixture %d trigger: %v", fixture.ID, err)
		}
		input, err := BuildIRCallEnvelope(ir, trigger)
		if err != nil {
			t.Fatalf("build fixture %d ABI envelope: %v", fixture.ID, err)
		}
		response, err := runtime.RunInstalledWithInputAndResponse(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", input)
		if err != nil {
			t.Fatalf("run preview Wasm for fixture %d: %v", fixture.ID, err)
		}
		var actualResponse abi.Response
		if err := json.Unmarshal(response, &actualResponse); err != nil {
			t.Fatalf("decode preview response for fixture %d: %v", fixture.ID, err)
		}
		if !actualResponse.OK || actualResponse.Call != "catalog.supplier.product.preview" {
			t.Fatalf("fixture %d preview response = %s", fixture.ID, response)
		}
		if !reflect.DeepEqual(actualResponse.Data, expectedData) {
			t.Errorf("fixture %d Wasm preview = %#v, want %#v", fixture.ID, actualResponse.Data, expectedData)
		}
	}
	if len(seen) != len(expectedByID) {
		t.Fatalf("Wasm fixtures exercised = %d, want %d", len(seen), len(expectedByID))
	}
	var productCountAfter int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM products WHERE shop_id = $1`, seed.ShopID).Scan(&productCountAfter); err != nil {
		t.Fatalf("count products after preview: %v", err)
	}
	if productCountAfter != productCountBefore {
		t.Fatalf("preview changed persistent product count from %d to %d", productCountBefore, productCountAfter)
	}
}
