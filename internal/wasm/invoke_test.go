package wasm

import (
	"bytes"
	"context"
	"encoding/json"
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

func TestCatalogProductNewestSeed(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.newest",
		"input":{"days":30,"limit":1}
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
	for _, input := range []string{
		`{"days":3651,"limit":1}`,
		`{"days":30,"limit":101}`,
		`{"days":30,"limit":0}`,
	} {
		raw := h.Invoke(context.Background(), tc, []byte(`{
			"abi":"euromni.abi.v1",
			"call":"catalog.product.newest",
			"input":`+input+`
		}`))
		if !bytes.Contains(raw, []byte(`"invalid"`)) {
			t.Fatalf("invalid query %s was not rejected: %s", input, raw)
		}
	}
}

func TestCatalogProductNewestDisabledWithZeroDays(t *testing.T) {
	h := testHost()
	tc := mustCtx(t, CapCatalog)
	raw := h.Invoke(context.Background(), tc, []byte(`{
		"abi":"euromni.abi.v1",
		"call":"catalog.product.newest",
		"input":{"days":0,"limit":8}
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
