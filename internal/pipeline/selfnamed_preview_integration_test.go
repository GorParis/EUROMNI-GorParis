//go:build integration

package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
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

	fixture := `{"id":842003,"name":"Synthetic Out-of-Stock Balm","description":"Synthetic unavailable stock case.","price":17.40,"volume":88,"weight":1001,"isInStock":false,"images":[],"image":"","tags":["synthetic","edge-case"]}`
	trigger, err := json.Marshal(map[string]string{"product_json": fixture})
	if err != nil {
		t.Fatalf("encode preview trigger: %v", err)
	}
	input, err := BuildIRCallEnvelope(ir, trigger)
	if err != nil {
		t.Fatalf("build preview ABI envelope: %v", err)
	}
	host := &wasm.Host{}

	var productCountBefore int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM products WHERE shop_id = $1`, seed.ShopID).Scan(&productCountBefore); err != nil {
		t.Fatalf("count products before preview: %v", err)
	}
	runtime := wasm.NewRuntime(host)
	t.Cleanup(runtime.Close)
	if err := runtime.LoadInstalled(ctx, moduleStore, seed.ShopID, seed.ModuleID, storedWasm); err != nil {
		t.Fatalf("load preview Wasm build: %v", err)
	}
	response, err := runtime.RunInstalledWithInputAndResponse(ctx, moduleStore, seed.ShopID, seed.ModuleID, "run", input)
	if err != nil {
		t.Fatalf("run preview Wasm: %v", err)
	}
	var actualResponse abi.Response
	if err := json.Unmarshal(response, &actualResponse); err != nil {
		t.Fatalf("decode preview Wasm response: %v", err)
	}
	if !actualResponse.OK {
		t.Fatalf("preview Wasm returned error: %s", response)
	}
	expectedData := map[string]any{
		"provider":           "selfnamed",
		"external_id":        "842003",
		"publication_status": "draft",
		"name":               "Synthetic Out-of-Stock Balm",
		"description":        "Synthetic unavailable stock case.",
		"locale":             nil,
		"price":              map[string]any{"amount": "17.4", "currency": nil},
		"inventory":          map[string]any{"status": "outofstock", "track_quantity": false},
		"weight":             map[string]any{"grams": float64(1001), "unit": nil},
		"volume":             map[string]any{"value": float64(88), "unit": nil},
		"tags":               []any{"synthetic", "edge-case"},
		"unresolved":         []any{"currency", "locale", "volume_unit"},
	}
	expectedJSON, err := json.Marshal(expectedData)
	if err != nil {
		t.Fatalf("encode expected preview: %v", err)
	}
	actualJSON, err := json.Marshal(actualResponse.Data)
	if err != nil {
		t.Fatalf("encode actual preview: %v", err)
	}
	if !bytes.Equal(actualJSON, expectedJSON) {
		t.Fatalf("Wasm preview data = %s, want %s", actualJSON, expectedJSON)
	}
	var productCountAfter int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM products WHERE shop_id = $1`, seed.ShopID).Scan(&productCountAfter); err != nil {
		t.Fatalf("count products after preview: %v", err)
	}
	if productCountAfter != productCountBefore {
		t.Fatalf("preview changed persistent product count from %d to %d", productCountBefore, productCountAfter)
	}
}
