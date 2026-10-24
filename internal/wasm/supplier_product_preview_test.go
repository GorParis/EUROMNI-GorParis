package wasm

import (
	"context"
	"encoding/json"
	"testing"

	"euromni/internal/abi"
)

func TestCatalogSupplierProductPreviewResultValidatesAndReturnsGuestMapping(t *testing.T) {
	host := &Host{}
	tc, err := trustedContext("e0e00000-0001-4000-8000-000000000001", "e0e00000-0001-4000-8000-000000000080", CapCatalog)
	if err != nil {
		t.Fatal(err)
	}
	request := []byte(`{"abi":"euromni.abi.v1","call":"catalog.supplier.product.preview.result","input":{"preview":{"provider":"selfnamed","external_id":"842003","publication_status":"draft","name":"Synthetic Out-of-Stock Balm","description":"Synthetic unavailable stock case.","locale":null,"price":{"amount":"17.4","currency":null},"inventory":{"status":"outofstock","track_quantity":false},"weight":{"grams":1001,"unit":null},"volume":{"value":88,"unit":null},"tags":["synthetic","edge-case"],"unresolved":["currency","locale","volume_unit"]}}}`)

	response := host.Invoke(context.Background(), tc, request)
	var decoded abi.Response
	if err := json.Unmarshal(response, &decoded); err != nil {
		t.Fatalf("decode result response: %v", err)
	}
	if !decoded.OK || decoded.Call != "catalog.supplier.product.preview.result" {
		t.Fatalf("result response = %s, want successful preview result", response)
	}
}

func TestCatalogSupplierProductPreviewResultRejectsInvalidGuestMapping(t *testing.T) {
	host := &Host{}
	tc, err := trustedContext("e0e00000-0001-4000-8000-000000000001", "e0e00000-0001-4000-8000-000000000080", CapCatalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		preview string
	}{
		{name: "missing fields", preview: `{}`},
		{name: "null tags", preview: `{"provider":"selfnamed","external_id":"842003","publication_status":"draft","name":"Product","description":"","locale":null,"price":{"amount":"17.4","currency":null},"inventory":{"status":"instock","track_quantity":false},"weight":{"grams":1,"unit":null},"volume":{"value":1,"unit":null},"tags":null,"unresolved":["currency","locale","volume_unit"]}`},
		{name: "null quantity flag", preview: `{"provider":"selfnamed","external_id":"842003","publication_status":"draft","name":"Product","description":"","locale":null,"price":{"amount":"17.4","currency":null},"inventory":{"status":"instock","track_quantity":null},"weight":{"grams":1,"unit":null},"volume":{"value":1,"unit":null},"tags":[],"unresolved":["currency","locale","volume_unit"]}`},
		{name: "non-normalized price", preview: `{"provider":"selfnamed","external_id":"842003","publication_status":"draft","name":"Product","description":"","locale":null,"price":{"amount":"17.40","currency":null},"inventory":{"status":"instock","track_quantity":false},"weight":{"grams":1,"unit":null},"volume":{"value":1,"unit":null},"tags":[],"unresolved":["currency","locale","volume_unit"]}`},
		{name: "unknown field", preview: `{"provider":"selfnamed","external_id":"842003","publication_status":"draft","name":"Product","description":"","locale":null,"price":{"amount":"17.4","currency":null},"inventory":{"status":"instock","track_quantity":false},"weight":{"grams":1,"unit":null},"volume":{"value":1,"unit":null},"tags":[],"unresolved":["currency","locale","volume_unit"],"write_to_catalog":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := []byte(`{"abi":"euromni.abi.v1","call":"catalog.supplier.product.preview.result","input":{"preview":` + test.preview + `}}`)
			response := host.Invoke(context.Background(), tc, request)
			var decoded abi.Response
			if err := json.Unmarshal(response, &decoded); err != nil {
				t.Fatalf("decode result response: %v", err)
			}
			if decoded.OK || decoded.Error == nil || decoded.Error.Code != abi.CodeInvalid {
				t.Fatalf("invalid result response = %s, want ABI invalid error", response)
			}
		})
	}
}
