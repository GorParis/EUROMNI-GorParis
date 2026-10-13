package abi

import "testing"

func TestParseEnvelopeRejectsUnknownFieldsAndOversize(t *testing.T) {
	if _, err := ParseEnvelope(nil); err == nil {
		t.Fatal("expected error")
	}
	big := make([]byte, MaxCallBytes+1)
	_, err := ParseEnvelope(big)
	if c, _ := GuestCode(err); c != CodeTooLarge {
		t.Fatalf("got %v", err)
	}
	_, err = ParseEnvelope([]byte(`{"abi":"euromni.abi.v1","call":"catalog.product.get","shop_id":"e0e00000-0001-4000-8000-000000000001","input":{"product_id":"e0e00000-0001-4000-8000-000000000040"}}`))
	if err == nil {
		t.Fatal("guest shop_id must be rejected")
	}
	if c, _ := GuestCode(err); c != CodeInvalid {
		t.Fatalf("code %s", c)
	}
}

func TestParseEnvelopeRejectsTrailingJSON(t *testing.T) {
	raw := []byte(`{"abi":"euromni.abi.v1","call":"catalog.product.list","input":{}} {"shop_id":"e0e00000-0001-4000-8000-000000000001"}`)
	if _, err := ParseEnvelope(raw); err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	} else if code, _ := GuestCode(err); code != CodeInvalid {
		t.Fatalf("error code = %q, want %q", code, CodeInvalid)
	}
}

func TestDecodeInputRejectsTrailingJSON(t *testing.T) {
	type input struct {
		ProductID string `json:"product_id"`
	}
	_, err := DecodeInput[input]([]byte(`{"product_id":"e0e00000-0001-4000-8000-000000000040"} {"extra":true}`))
	if err == nil {
		t.Fatal("expected trailing JSON to be rejected")
	}
	if code, _ := GuestCode(err); code != CodeInvalid {
		t.Fatalf("error code = %q, want %q", code, CodeInvalid)
	}
}

func TestNormalizeUUID(t *testing.T) {
	id, err := NormalizeUUID("E0E00000-0001-4000-8000-000000000040")
	if err != nil || id != "e0e00000-0001-4000-8000-000000000040" {
		t.Fatalf("%s %v", id, err)
	}
	if _, err := NormalizeUUID("not-a-uuid"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := NormalizeUUID("e0e00000-0001-4000-8000-000000000040'; DROP TABLE products;--"); err == nil {
		t.Fatal("expected error")
	}
}
