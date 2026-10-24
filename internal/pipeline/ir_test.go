package pipeline

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"euromni/internal/abi"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

func TestValidateIRDerivesCapabilitiesFromHostMapping(t *testing.T) {
	ir := testCatalogIR()
	validated, err := ValidateIR(ir)
	if err != nil {
		t.Fatalf("ValidateIR() returned error: %v", err)
	}
	if len(validated.Capabilities) != 1 || validated.Capabilities[0] != "catalog" {
		t.Fatalf("capabilities = %#v, want catalog derived from host mapping", validated.Capabilities)
	}
}

func TestLowerPHPAnalysisToIRSupportsOneWooCommerceHook(t *testing.T) {
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
		"manifest.json":  `{"name":"demo.catalog-reader","version":"1.0.0","platform":"woocommerce","shop":"atelier-demo"}`,
		"src/plugin.php": string(source),
	})
	provenance, err := NewPackageProvenance(source, archive, testSourceProvenanceMetadata("woocommerce"))
	if err != nil {
		t.Fatalf("NewSourceProvenance() returned error: %v", err)
	}
	ir, err := LowerPHPAnalysisToIR("demo.catalog-reader", analysis, provenance)
	if err != nil {
		t.Fatalf("LowerPHPAnalysisToIR() returned error: %v", err)
	}
	if ir.Platform != "woocommerce" || len(ir.Triggers) != 1 || ir.Triggers[0].Kind != "hook" || ir.Triggers[0].Name != "woocommerce_product_query" {
		t.Fatalf("IR trigger = %#v, want the analyzed WooCommerce hook", ir.Triggers)
	}
	if len(ir.Operations) != 1 || ir.Operations[0].Call != "catalog.product.list" {
		t.Fatalf("IR operations = %#v, want one catalog.product.list call", ir.Operations)
	}
}

func TestBuildWooCommerceSortingArchiveUsesPinnedSource(t *testing.T) {
	archive, err := os.ReadFile("../../pipeline/fixtures/woocommerce-extra-product-sorting-options.zip")
	if err != nil {
		t.Fatalf("read pinned WooCommerce archive: %v", err)
	}
	ir, wasmBytes, err := BuildWooCommerceSortingArchive(archive)
	if err != nil {
		t.Fatalf("BuildWooCommerceSortingArchive() returned error: %v", err)
	}
	if ir.Module != WooCommerceSortingModuleName || ir.Provenance.ModuleVersion != WooCommerceSortingModuleVersion {
		t.Fatalf("module provenance = %q %q, want pinned module version", ir.Module, ir.Provenance.ModuleVersion)
	}
	if ir.Provenance.ArchiveSHA256 != wooCommerceSortingArchiveSHA256 ||
		ir.Provenance.SourceSHA256 != WooCommerceSortingSourceSHA256 {
		t.Fatalf("source provenance does not match the pinned archive: %+v", ir.Provenance)
	}
	if len(ir.Triggers) != 1 || ir.Triggers[0].Name != "woocommerce_get_catalog_ordering_args" ||
		ir.Triggers[0].Fields["locale"] != "string" || ir.Triggers[0].Fields["orderby"] != "string" {
		t.Fatalf("trigger = %#v, want the bounded WooCommerce sorting hook", ir.Triggers)
	}
	if len(ir.Operations) != 1 || ir.Operations[0].Call != "catalog.product.sort" ||
		ir.Operations[0].Input["orderby"].TriggerField != "orderby" ||
		ir.Operations[0].Input["locale"].TriggerField != "locale" {
		t.Fatalf("operation = %#v, want dynamic catalog sorting inputs", ir.Operations)
	}
	assertOnlyEUROMNICallImport(t, wasmBytes)
	if _, _, err := BuildWooCommerceSortingArchive(append([]byte(nil), archive[:len(archive)-1]...)); err == nil {
		t.Fatal("BuildWooCommerceSortingArchive() accepted a modified package")
	}
}

func TestLowerPHPAnalysisToIRRejectsUnmappedHooks(t *testing.T) {
	analysis := &PHPAnalysis{Platform: "woocommerce", Hooks: []string{"woocommerce_after_calculate_totals"}}
	if _, err := LowerPHPAnalysisToIR("demo.module", analysis, testSourceProvenance("woocommerce")); err == nil {
		t.Fatal("LowerPHPAnalysisToIR() accepted a hook without an explicit lowering rule")
	}
}

func TestLowerPHPAnalysisToIRRejectsMismatchedSourceProvenance(t *testing.T) {
	source := []byte(`<?php add_action('woocommerce_product_query', function () {});`)
	analysis, err := AnalyzePHPSource(string(source))
	if err != nil {
		t.Fatalf("AnalyzePHPSource() returned error: %v", err)
	}
	provenance, err := NewPackageProvenance([]byte("different source"), []byte("package archive"), testSourceProvenanceMetadata("woocommerce"))
	if err != nil {
		t.Fatalf("NewSourceProvenance() returned error: %v", err)
	}
	if _, err := LowerPHPAnalysisToIR("demo.module", analysis, provenance); err == nil {
		t.Fatal("LowerPHPAnalysisToIR() accepted provenance for different source bytes")
	}
}

func TestSourceProvenanceTracksAndVerifiesSource(t *testing.T) {
	source := []byte("module source fixture")
	archive := []byte("original package archive")
	provenance, err := NewPackageProvenance(source, archive, testSourceProvenanceMetadata("woocommerce"))
	if err != nil {
		t.Fatalf("NewSourceProvenance() returned error: %v", err)
	}
	if err := provenance.Validate("woocommerce"); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
	if err := provenance.VerifySource(source); err != nil {
		t.Fatalf("VerifySource() rejected matching bytes: %v", err)
	}
	if err := provenance.VerifyArchive(archive); err != nil {
		t.Fatalf("VerifyArchive() rejected matching bytes: %v", err)
	}
	if err := provenance.VerifySource([]byte("modified module source")); err == nil {
		t.Fatal("VerifySource() accepted bytes that do not match the recorded hash")
	}
	if err := provenance.VerifyArchive([]byte("modified archive")); err == nil {
		t.Fatal("VerifyArchive() accepted bytes that do not match the recorded hash")
	}
	if _, err := NewSourceProvenance(source, SourceProvenance{SourceSHA256: strings.Repeat("0", 64)}); err == nil {
		t.Fatal("NewSourceProvenance() accepted a conflicting source hash")
	}
}

func TestValidateIRRejectsMismatchedProvenance(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ModuleIR)
	}{
		{name: "adapter platform mismatch", edit: func(ir *ModuleIR) { ir.Provenance.Adapter = "prestashop" }},
		{name: "missing platform version", edit: func(ir *ModuleIR) { ir.Provenance.PlatformVersion = "" }},
		{name: "malformed source hash", edit: func(ir *ModuleIR) { ir.Provenance.SourceSHA256 = "abc" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ir := testCatalogIR()
			test.edit(&ir)
			if _, err := ValidateIR(ir); err == nil {
				t.Fatal("ValidateIR() accepted invalid source provenance")
			}
		})
	}
}

func TestValidateIRRejectsUnsupportedTriggerDescriptors(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ModuleIR)
	}{
		{
			name: "unknown trigger kind",
			edit: func(ir *ModuleIR) { ir.Triggers[0].Kind = "filesystem" },
		},
		{
			name: "untrimmed trigger name",
			edit: func(ir *ModuleIR) { ir.Triggers[0].Name += " " },
		},
		{
			name: "invalid trigger field type",
			edit: func(ir *ModuleIR) { ir.Triggers[0].Fields = map[string]string{"product_id": "json"} },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ir := testDynamicCatalogIR()
			test.edit(&ir)
			if _, err := ValidateIR(ir); err == nil {
				t.Fatal("ValidateIR() accepted an unsupported trigger descriptor")
			}
		})
	}
}

func TestValidateIRRejectsUnknownCallsAndInvalidInputs(t *testing.T) {
	tests := []struct {
		name string
		edit func(*ModuleIR)
	}{
		{
			name: "unknown host call",
			edit: func(ir *ModuleIR) { ir.Operations[0].Call = "filesystem.read" },
		},
		{
			name: "mapped but unsupported schema",
			edit: func(ir *ModuleIR) { ir.Operations[0].Call = "payment.initiate" },
		},
		{
			name: "invalid product identifier",
			edit: func(ir *ModuleIR) {
				ir.Operations[0].Input["product_id"] = IRValue{Literal: json.RawMessage(`"not-a-uuid"`)}
			},
		},
		{
			name: "unrecognized field",
			edit: func(ir *ModuleIR) {
				ir.Operations[0].Input["sql"] = IRValue{Literal: json.RawMessage(`"select"`)}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ir := testCatalogIR()
			test.edit(&ir)
			if _, err := ValidateIR(ir); err == nil {
				t.Fatal("ValidateIR() accepted an invalid operation")
			}
		})
	}
}

func TestCompileIRToWasmIsDeterministic(t *testing.T) {
	ir := testCatalogIR()
	first, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("CompileIRToWasm() returned error: %v", err)
	}
	second, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("second CompileIRToWasm() returned error: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical IR produced different Wasm bytes")
	}
	if len(first) < 4 || !bytes.Equal(first[:4], []byte{0, 'a', 's', 'm'}) {
		t.Fatalf("compiled bytes do not have the Wasm magic header: %x", first[:min(4, len(first))])
	}
	assertOnlyEUROMNICallImport(t, first)

	wat, err := CompileIRToWAT(ir)
	if err != nil {
		t.Fatalf("CompileIRToWAT() returned error: %v", err)
	}
	if !strings.Contains(wat, `\"catalog.product.get\"`) || !strings.Contains(wat, `(import "euromni" "call"`) {
		t.Fatalf("WAT does not call the expected ABI: %s", wat)
	}
	if !strings.Contains(wat, `(memory (export "memory") 17)`) || !strings.Contains(wat, `(i32.const 1048576)`) {
		t.Fatalf("WAT does not reserve memory for the ABI response: %s", wat)
	}
}

func TestCompileIRToWasmRequiresSpecializedSelfnamedPreviewBuild(t *testing.T) {
	ir := testSelfnamedPreviewIR()
	if _, err := CompileIRToWasm(ir); err == nil {
		t.Fatal("CompileIRToWasm() accepted Selfnamed preview IR")
	}
}

func TestCompileSelfnamedPreviewRejectsIROutsideGuestContract(t *testing.T) {
	ir := testSelfnamedPreviewIR()
	ir.Triggers[0].Name = "other.trigger"
	if _, err := CompileSelfnamedPreviewIRToWasm(ir); err == nil {
		t.Fatal("CompileSelfnamedPreviewIRToWasm() accepted an unrelated trigger")
	}
}

func TestCompileIRToWATCanonicalizesInput(t *testing.T) {
	ir := testCatalogIR()
	ir.Operations[0].Input["product_id"] = IRValue{Literal: json.RawMessage(` "e0e00000-0001-4000-8000-000000000040" `)}
	first, err := CompileIRToWAT(ir)
	if err != nil {
		t.Fatal(err)
	}
	ir.Operations[0].Input["product_id"] = IRValue{Literal: json.RawMessage(`"e0e00000-0001-4000-8000-000000000040"`)}
	second, err := CompileIRToWAT(ir)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("semantically identical input produced different WAT:\n%s\n%s", first, second)
	}
}

func TestBuildIRCallEnvelopeResolvesDynamicTriggerField(t *testing.T) {
	ir := testDynamicCatalogIR()
	envelope, err := BuildIRCallEnvelope(ir, []byte(`{"product_id":"e0e00000-0001-4000-8000-000000000040"}`))
	if err != nil {
		t.Fatalf("BuildIRCallEnvelope() returned error: %v", err)
	}
	parsed, err := abi.ParseEnvelope(envelope)
	if err != nil {
		t.Fatalf("generated ABI envelope is invalid: %v", err)
	}
	if parsed.Call != "catalog.product.get" {
		t.Fatalf("call = %q, want catalog.product.get", parsed.Call)
	}
	input, err := abi.DecodeInput[struct {
		ProductID string `json:"product_id"`
	}](parsed.Input)
	if err != nil || input.ProductID != "e0e00000-0001-4000-8000-000000000040" {
		t.Fatalf("resolved input = %+v, error = %v", input, err)
	}
}

func TestBuildIRCallEnvelopeAcceptsJSONWhitespace(t *testing.T) {
	payload := []byte(" \n { \"product_id\": \"e0e00000-0001-4000-8000-000000000040\" } \t")
	if _, err := BuildIRCallEnvelope(testDynamicCatalogIR(), payload); err != nil {
		t.Fatalf("BuildIRCallEnvelope() rejected valid JSON whitespace: %v", err)
	}
}

func TestBuildIRCallEnvelopeRejectsInvalidTriggerPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "missing required field", payload: `{}`},
		{name: "unknown field", payload: `{"product_id":"e0e00000-0001-4000-8000-000000000040","extra":"x"}`},
		{name: "invalid UUID", payload: `{"product_id":"not-a-uuid"}`},
		{name: "wrong type", payload: `{"product_id":42}`},
		{name: "not an object", payload: `[]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := BuildIRCallEnvelope(testDynamicCatalogIR(), []byte(test.payload)); err == nil {
				t.Fatal("BuildIRCallEnvelope() accepted invalid trigger input")
			}
		})
	}
}

func TestCompileDynamicIRToWasmAcceptsRuntimePayload(t *testing.T) {
	ir := testDynamicCatalogIR()
	wasmBytes, err := CompileIRToWasm(ir)
	if err != nil {
		t.Fatalf("CompileIRToWasm() returned error: %v", err)
	}
	wat, err := CompileIRToWAT(ir)
	if err != nil {
		t.Fatalf("CompileIRToWAT() returned error: %v", err)
	}
	if len(wasmBytes) < 4 || !bytes.Equal(wasmBytes[:4], []byte{0, 'a', 's', 'm'}) {
		t.Fatalf("compiled bytes do not have the Wasm magic header: %x", wasmBytes[:min(4, len(wasmBytes))])
	}
	assertOnlyEUROMNICallImport(t, wasmBytes)
	if !strings.Contains(wat, `(param $input_ptr i32) (param $input_len i32)`) || strings.Contains(wat, "e0e00000") {
		t.Fatalf("dynamic Wasm should accept runtime input without embedding its value: %s", wat)
	}
}

func assertOnlyEUROMNICallImport(t *testing.T, wasmBytes []byte) {
	t.Helper()

	engine := wasmtime.NewEngine()
	defer engine.Close()
	module, err := wasmtime.NewModule(engine, wasmBytes)
	if err != nil {
		t.Fatalf("inspect compiled Wasm imports: %v", err)
	}
	defer module.Close()

	imports := module.Imports()
	defer func() {
		for _, imported := range imports {
			imported.Close()
		}
	}()
	if len(imports) != 1 {
		t.Fatalf("compiled Wasm has %d imports, want only euromni.call", len(imports))
	}
	name := imports[0].Name()
	if imports[0].Module() != "euromni" || name == nil || *name != "call" {
		importName := "<nil>"
		if name != nil {
			importName = *name
		}
		t.Fatalf("compiled Wasm import is %s.%s, want euromni.call", imports[0].Module(), importName)
	}
}

func testCatalogIR() ModuleIR {
	return ModuleIR{
		Version:    IRVersion,
		Platform:   "woocommerce",
		Module:     "demo.catalog-reader",
		Provenance: testSourceProvenance("woocommerce"),
		Triggers:   []IRTrigger{{Kind: "hook", Name: "woocommerce_product_query"}},
		Operations: []IROperation{{
			Call:  "catalog.product.get",
			Input: map[string]IRValue{"product_id": {Literal: json.RawMessage(`"e0e00000-0001-4000-8000-000000000040"`)}},
		}},
	}
}

func testDynamicCatalogIR() ModuleIR {
	return ModuleIR{
		Version:    IRVersion,
		Platform:   "woocommerce",
		Module:     "demo.catalog-reader",
		Provenance: testSourceProvenance("woocommerce"),
		Triggers: []IRTrigger{{
			Kind:   "hook",
			Name:   "woocommerce_product_query",
			Fields: map[string]string{"product_id": "uuid"},
		}},
		Operations: []IROperation{{
			Call:  "catalog.product.get",
			Input: map[string]IRValue{"product_id": {TriggerField: "product_id"}},
		}},
	}
}

func testSelfnamedPreviewIR() ModuleIR {
	return ModuleIR{
		Version:    IRVersion,
		Platform:   "woocommerce",
		Module:     "selfnamed-catalog-preview",
		Provenance: testSourceProvenance("woocommerce"),
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
}

func testSourceProvenance(platform string) SourceProvenance {
	provenance := testSourceProvenanceMetadata(platform)
	provenance.SourceSHA256 = strings.Repeat("a", 64)
	provenance.ArchiveSHA256 = strings.Repeat("b", 64)
	return provenance
}

func testSourceProvenanceMetadata(platform string) SourceProvenance {
	return SourceProvenance{
		SourceReference: "test-fixture",
		ModuleVersion:   "1.0.0",
		PlatformVersion: "unknown",
		Adapter:         platform,
		AdapterVersion:  "1.0.0",
	}
}
