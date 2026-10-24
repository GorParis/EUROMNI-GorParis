package wasm

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"euromni/internal/abi"
	"euromni/internal/seed"
	"euromni/internal/store"
)

func testHost() *Host {
	memory := store.NewMemory()
	return &Host{Products: memory, Categories: memory}
}

func mustCtx(t *testing.T, caps ...Capability) TrustedContext {
	t.Helper()
	tc, err := trustedContext(seed.ShopID, seed.ModuleID, caps...)
	if err != nil {
		t.Fatal(err)
	}
	tc.config = []byte(`{"days":30,"limit":8}`)
	return tc
}

func TestCatalogProductListSeed(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.list",
		"input":{}
	}`))
	var resp abi.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("not ok: %s", raw)
	}
	b, _ := json.Marshal(resp.Data)
	if !bytes.Contains(b, []byte(`"cabas-lin"`)) || !bytes.Contains(b, []byte(`"ceinture-cuir"`)) {
		t.Fatalf("unexpected list %s", b)
	}
}

func TestCatalogProductSortMatchesSupportedWooCommerceOptions(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	tests := []struct {
		orderby  string
		want     []string
		variants map[string]int
	}{
		{
			orderby: "alphabetical",
			want:    []string{"bougie-cedre", "cabas-lin", "ceinture-cuir", "chemise-lin", "tasse-gres"},
			variants: map[string]int{
				"bougie-cedre": 1, "cabas-lin": 1, "ceinture-cuir": 3, "chemise-lin": 3, "tasse-gres": 1,
			},
		},
		{
			orderby: "reverse_alpha",
			want:    []string{"tasse-gres", "chemise-lin", "ceinture-cuir", "cabas-lin", "bougie-cedre"},
			variants: map[string]int{
				"bougie-cedre": 1, "cabas-lin": 1, "ceinture-cuir": 3, "chemise-lin": 3, "tasse-gres": 1,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.orderby, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"locale": "fr-FR", "orderby": test.orderby})
			if err != nil {
				t.Fatal(err)
			}
			raw := h.Invoke(context.Background(), tc, sortingEnvelope(t, input))
			var response abi.Response
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if !response.OK {
				t.Fatalf("sorting call failed: %s", raw)
			}
			encoded, err := json.Marshal(response.Data)
			if err != nil {
				t.Fatal(err)
			}
			var products []store.ProductView
			if err := json.Unmarshal(encoded, &products); err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(products))
			for i, product := range products {
				got[i] = product.Slug
				if len(product.Variants) != test.variants[product.Slug] {
					t.Errorf("%s variants = %d, want %d", product.Slug, len(product.Variants), test.variants[product.Slug])
				}
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("sorted products = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCatalogProductSortByStockUsesWooCommerceFallbackOrder(t *testing.T) {
	h := &Host{Products: sortingProductReader{
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
	}}
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, sortingEnvelope(t, []byte(`{"locale":"fr-FR","orderby":"by_stock"}`)))
	var response abi.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("stock sorting call failed: %s", raw)
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatal(err)
	}
	var products []store.ProductView
	if err := json.Unmarshal(encoded, &products); err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(products))
	for i, product := range products {
		got[i] = product.Slug
	}
	want := []string{"abricot", "citron", "banane"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stock-sorted products = %#v, want %#v", got, want)
	}
}

func TestCatalogProductSortByStockRejectsUnsupportedInventoryShapes(t *testing.T) {
	tests := []struct {
		name     string
		variants []store.VariantView
	}{
		{name: "no variant"},
		{name: "multiple variants", variants: []store.VariantView{{Inventory: store.InventoryView{Track: true}}, {Inventory: store.InventoryView{Track: true}}}},
		{name: "untracked stock", variants: []store.VariantView{{Inventory: store.InventoryView{Quantity: 5}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := &Host{Products: sortingProductReader{{ID: "product", Slug: "product", Variants: test.variants}}}
			raw := h.Invoke(context.Background(), mustCtx(t, CapCatalog), sortingEnvelope(t, []byte(`{"locale":"fr-FR","orderby":"by_stock"}`)))
			var response abi.Response
			if err := json.Unmarshal(raw, &response); err != nil {
				t.Fatal(err)
			}
			if response.OK || response.Error == nil || response.Error.Code != abi.CodeInvalid {
				t.Fatalf("unsupported inventory shape was not rejected: %s", raw)
			}
		})
	}
}

func TestCatalogProductSortRejectsUnsupportedOptionsAndLocales(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	for _, input := range []string{
		`{"locale":"fr-FR","orderby":"review_count"}`,
		`{"locale":"es-ES","orderby":"alphabetical"}`,
	} {
		raw := h.Invoke(context.Background(), tc, sortingEnvelope(t, []byte(input)))
		var response abi.Response
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		if response.OK || response.Error == nil || response.Error.Code != abi.CodeInvalid {
			t.Fatalf("unsupported sorting input %s was not rejected: %s", input, raw)
		}
	}
}

type sortingProductReader []store.ProductView

func (p sortingProductReader) GetProduct(context.Context, string, string) (*store.ProductView, error) {
	return nil, store.ErrNotFound
}

func (p sortingProductReader) ListProducts(context.Context, string) ([]store.ProductView, error) {
	return append([]store.ProductView(nil), p...), nil
}

func sortingEnvelope(t *testing.T, input []byte) []byte {
	t.Helper()
	envelope, err := json.Marshal(struct {
		ABI   string          `json:"abi"`
		Call  string          `json:"call"`
		Input json.RawMessage `json:"input"`
	}{ABI: abi.Version, Call: "catalog.product.sort", Input: input})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestCatalogProductNewestSeed(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.newest",
		"input":{}
	}`))
	var resp abi.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("not ok: %s", raw)
	}
	b, _ := json.Marshal(resp.Data)
	if !bytes.Contains(b, []byte(`"id":"e0e00000-0001-4000-8000-000000000056"`)) {
		t.Fatalf("new products query did not return the newest seeded product: %s", b)
	}
}

func TestCatalogProductNewestRejectsInvalidBounds(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	for _, config := range []string{
		`{"days":3651,"limit":1}`,
		`{"days":30,"limit":101}`,
		`{"days":30,"limit":0}`,
		`{"days":30.5,"limit":8}`,
		`{"days":-1,"limit":8}`,
		`{"days":null,"limit":8}`,
	} {
		tc.config = []byte(config)
		raw := h.Invoke(context.Background(), tc, []byte(`{
			"abi":"euromni.abi.v1",
			"call":"catalog.product.newest",
			"input":{}
		}`))
		if !bytes.Contains(raw, []byte(`"invalid"`)) {
			t.Fatalf("invalid module configuration %s was not rejected: %s", config, raw)
		}
	}
}

func TestCatalogProductNewestRequiresModuleSettings(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	tc.config = []byte(`{"limit":8}`)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.newest",
		"input":{}
	}`))
	if !bytes.Contains(raw, []byte(`"invalid"`)) {
		t.Fatalf("missing module setting was not rejected explicitly: %s", raw)
	}
}

func TestCatalogProductNewestDisabledWithZeroDays(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	tc.config = []byte(`{"days":0,"limit":8}`)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.newest",
		"input":{}
	}`))
	var resp abi.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("disabled new-products widget was not accepted: %s", raw)
	}
	if products, ok := resp.Data.([]any); !ok || len(products) != 0 {
		t.Fatalf("disabled new-products result = %#v, want an empty list", resp.Data)
	}
}

func TestCatalogProductGetSeed(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.get",
		"id":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}
	}`))
	var resp abi.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}

	if !resp.OK {
		t.Fatalf("not ok: %s", raw)
	}
	b, _ := json.Marshal(resp.Data)
	if !bytes.Contains(b, []byte(`"cabas-lin"`)) || !bytes.Contains(b, []byte(`"89.0000"`)) {
		t.Fatalf("unexpected data %s", b)
	}
}

func TestCatalogCategoryListSeed(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.category.list",
		"input":{}
	}`))
	var resp abi.Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}

	if !resp.OK {
		t.Fatalf("not ok: %s", raw)
	}
	b, _ := json.Marshal(resp.Data)
	if !bytes.Contains(b, []byte(`"sacs"`)) {
		t.Fatalf("unexpected category list %s", b)
	}
}

func TestCatalogProductGetDeniedWithoutCapability(t *testing.T) {
	h := testHost()
	tc := mustCtx(t) // no caps
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.get",
		"input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}
	}`))
	if !bytes.Contains(raw, []byte(`"denied"`)) {
		t.Fatalf("%s", raw)
	}
}

func TestCatalogProductGetIsolatedByShop(t *testing.T) {
	h := testHost()
	tc, err := trustedContext(seed.OtherShop, seed.ModuleID, CapCatalog)
	if err != nil {
		t.Fatal(err)
	}
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.get",
		"input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}
	}`))
	if !bytes.Contains(raw, []byte(`"not_found"`)) {
		t.Fatalf("wrong shop must look like missing, got %s", raw)
	}
}

func TestCatalogProductGetRejectsSQLInjectionID(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.get",
		"input":{"product_id":"e0e00000-0001-4000-8000-000000000040 OR 1=1"}
	}`))
	if !bytes.Contains(raw, []byte(`"invalid"`)) {
		t.Fatalf("%s", raw)
	}
}

func TestUnimplementedCallStillChecksCapability(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"order.place",
		"input":{}
	}`))
	if !bytes.Contains(raw, []byte(`"denied"`)) {
		t.Fatalf("order.place without order cap: %s", raw)
	}
}

func TestSQLErrorsAreNotLeaked(t *testing.T) {
	h := &Host{Products: failingProducts{}}
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.get",
		"input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}
	}`))
	if bytes.Contains(bytes.ToLower(raw), []byte("password")) || strings.Contains(string(raw), "pq:") {
		t.Fatalf("leaked: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"internal"`)) {
		t.Fatalf("%s", raw)
	}
}

type failingProducts struct{}

func (failingProducts) GetProduct(context.Context, string, string) (*store.ProductView, error) {
	return nil, errSecret("pq: password authentication failed")
}

func (failingProducts) ListProducts(context.Context, string) ([]store.ProductView, error) {
	return nil, errSecret("pq: password authentication failed")
}

type errSecret string

func (e errSecret) Error() string { return string(e) }

func TestSelfnamedCatalogProductPreview(t *testing.T) {
	h := &Host{}
	tc := mustCtx(t, CapCatalog)
	productJSON := `{"id":842002,"name":"Synthetic Catalog Balm","description":"Synthetic description used to characterize product mapping.","price":28.82,"volume":176,"weight":176,"isInStock":true,"images":[],"image":"","tags":[]}`
	encodedProduct, err := json.Marshal(productJSON)
	if err != nil {
		t.Fatal(err)
	}
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.supplier.product.preview",
		"input":{"product_json":`+string(encodedProduct)+`}
	}`))
	var response abi.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if !response.OK {
		t.Fatalf("preview failed: %s", raw)
	}
	encoded, err := json.Marshal(response.Data)
	if err != nil {
		t.Fatal(err)
	}
	var preview supplierProductPreview
	if err := json.Unmarshal(encoded, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Provider != "selfnamed" ||
		preview.ExternalID != "842002" ||
		preview.PublicationStatus != "draft" ||
		preview.Name != "Synthetic Catalog Balm" ||
		preview.Price.Amount != "28.82" ||
		preview.Price.Currency != nil ||
		preview.Locale != nil ||
		preview.Weight.Grams != 176 ||
		preview.Weight.Unit != nil ||
		preview.Volume.Value != 176 ||
		preview.Volume.Unit != nil ||
		preview.Inventory.Status != "instock" ||
		preview.Inventory.Track ||
		len(preview.Unresolved) != 3 {
		t.Fatalf("preview = %+v, missing expected normalized or unresolved fields", preview)
	}
}

func TestSelfnamedCatalogProductPreviewRejectsInvalidInput(t *testing.T) {
	h := &Host{}
	tc := mustCtx(t, CapCatalog)
	tests := []struct {
		name    string
		product string
	}{
		{name: "invalid JSON", product: `{"id":`},
		{name: "string ID", product: `{"id":"1","name":"Test","price":1,"volume":0,"weight":0,"isInStock":true}`},
		{name: "string price", product: `{"id":1,"name":"Test","price":"1","volume":0,"weight":0,"isInStock":true}`},
		{name: "missing stock status", product: `{"id":1,"name":"Test","price":1,"volume":0,"weight":0}`},
		{name: "invalid price", product: `{"id":1,"name":"Test","price":-1,"volume":0,"weight":0,"isInStock":true}`},
		{name: "unsupported decimal exponent", product: `{"id":1,"name":"Test","price":1e2,"volume":0,"weight":0,"isInStock":true}`},
		{name: "negative weight", product: `{"id":1,"name":"Test","price":1,"volume":0,"weight":-1,"isInStock":true}`},
		{name: "oversized nested input", product: strings.Repeat(" ", maxSupplierProductJSONBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := json.Marshal(map[string]string{"product_json": test.product})
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(struct {
				ABI   string          `json:"abi"`
				Call  string          `json:"call"`
				Input json.RawMessage `json:"input"`
			}{ABI: abi.Version, Call: "catalog.supplier.product.preview", Input: request})
			if err != nil {
				t.Fatal(err)
			}
			responseBytes := h.Invoke(context.Background(), tc, raw)
			var response abi.Response
			if err := json.Unmarshal(responseBytes, &response); err != nil {
				t.Fatal(err)
			}
			if response.OK || response.Error == nil || response.Error.Code != abi.CodeInvalid {
				t.Fatalf("invalid supplier input response = %s, want invalid", responseBytes)
			}
		})
	}
}

func TestSelfnamedCatalogProductPreviewRequiresCatalogCapability(t *testing.T) {
	h := &Host{}
	tc := mustCtx(t)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.supplier.product.preview",
		"input":{"product_json":"{}"}
	}`))
	if !bytes.Contains(raw, []byte(`"denied"`)) {
		t.Fatalf("preview without catalog capability was not denied: %s", raw)
	}
}

func TestNormalizeSupplierDecimal(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{input: "100", want: "100"},
		{input: "17.40", want: "17.4"},
		{input: "0.10", want: "0.1"},
		{input: "0.0000", want: "0"},
	} {
		t.Run(test.input, func(t *testing.T) {
			if got := normalizeSupplierDecimal(test.input); got != test.want {
				t.Fatalf("normalizeSupplierDecimal(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
