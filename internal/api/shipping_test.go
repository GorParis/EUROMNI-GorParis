package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"euromni/internal/store"
)

func TestPublicShippingOptionsByCountry(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() { SetProductReader(originalReader) })
	mux := http.NewServeMux()
	Register(mux)

	france := httptest.NewRecorder()
	mux.ServeHTTP(france, httptest.NewRequest(http.MethodGet, "/v1/shipping-options?country=fr", nil))
	if france.Code != http.StatusOK || !strings.Contains(france.Body.String(), `"carrier_code":"colissimo"`) ||
		!strings.Contains(france.Body.String(), `"amount":"6.9000"`) {
		t.Fatalf("France shipping options status/body = %d/%s", france.Code, france.Body)
	}
	unsupported := httptest.NewRecorder()
	mux.ServeHTTP(unsupported, httptest.NewRequest(http.MethodGet, "/v1/shipping-options?country=CA", nil))
	if unsupported.Code != http.StatusOK {
		t.Fatalf("unsupported destination status/body = %d/%s", unsupported.Code, unsupported.Body)
	}
	var body struct {
		Data []store.ShippingOption `json:"data"`
	}
	if err := json.NewDecoder(unsupported.Body).Decode(&body); err != nil {
		t.Fatalf("decode unsupported destination: %v", err)
	}
	if len(body.Data) != 0 {
		t.Fatalf("shipping options for unsupported destination = %+v, want empty", body.Data)
	}
	invalid := httptest.NewRecorder()
	mux.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/v1/shipping-options?country=France", nil))
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid country status/body = %d/%s, want 400", invalid.Code, invalid.Body)
	}
}

func TestValidateShippingConfig(t *testing.T) {
	valid := func() store.ShippingConfig {
		return store.ShippingConfig{
			Carriers: []store.ShippingCarrier{{Code: "colissimo", Name: "Colissimo", Active: true}},
			Zones: []store.ShippingZone{{
				Code: "france", Name: "France", CountryCodes: []string{"FR"}, Active: true,
			}},
			Rates: []store.ShippingRate{{
				CarrierCode: "colissimo", ZoneCode: "france", Amount: "6.90", Currency: "EUR", Active: true,
			}},
		}
	}

	tests := []struct {
		name   string
		mutate func(*store.ShippingConfig)
		want   string
	}{
		{name: "valid configuration"},
		{
			name: "duplicate carrier",
			mutate: func(config *store.ShippingConfig) {
				config.Carriers = append(config.Carriers, config.Carriers[0])
			},
			want: "carrier codes must be unique",
		},
		{
			name: "overlapping countries",
			mutate: func(config *store.ShippingConfig) {
				config.Zones = append(config.Zones, store.ShippingZone{
					Code: "france-2", Name: "France bis", CountryCodes: []string{"FR"}, Active: true,
				})
			},
			want: "a country cannot belong to more than one shipping zone",
		},
		{
			name: "unknown carrier",
			mutate: func(config *store.ShippingConfig) {
				config.Rates[0].CarrierCode = "unknown"
			},
			want: "each rate must refer to a configured carrier",
		},
		{
			name: "invalid amount",
			mutate: func(config *store.ShippingConfig) {
				config.Rates[0].Amount = "-1.00"
			},
			want: "shipping amounts must be non-negative numbers with at most four decimal places",
		},
		{
			name: "invalid currency",
			mutate: func(config *store.ShippingConfig) {
				config.Rates[0].Currency = "eur"
			},
			want: "currencies must use three uppercase letters",
		},
		{
			name: "valid adjacent weight and subtotal bands",
			mutate: func(config *store.ShippingConfig) {
				config.Rates[0].Mode = "weight_value"
				config.Rates[0].Amount = ""
				config.Rates[0].Rules = []store.ShippingRateRule{
					{MinWeightGrams: 0, MaxWeightGrams: int64Pointer(1000), MinSubtotal: "0", MaxSubtotal: stringPointer("50"), Amount: "5.90"},
					{MinWeightGrams: 0, MaxWeightGrams: int64Pointer(1000), MinSubtotal: "50", Amount: "4.90"},
					{MinWeightGrams: 1000, MinSubtotal: "0", Amount: "8.90"},
				}
			},
		},
		{
			name: "overlapping weight and subtotal rules",
			mutate: func(config *store.ShippingConfig) {
				config.Rates[0].Mode = "weight_value"
				config.Rates[0].Rules = []store.ShippingRateRule{
					{MinWeightGrams: 0, MaxWeightGrams: int64Pointer(1000), MinSubtotal: "0", Amount: "5.90"},
					{MinWeightGrams: 500, MinSubtotal: "50", Amount: "4.90"},
				}
			},
			want: "weight_value rules cannot overlap",
		},
		{
			name: "empty weight matrix",
			mutate: func(config *store.ShippingConfig) {
				config.Rates[0].Mode = "weight_value"
			},
			want: "weight_value rates require 1 to 200 rules",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid()
			if test.mutate != nil {
				test.mutate(&config)
			}

			got := validateShippingConfig(&config)
			if got != test.want {
				t.Fatalf("validation error = %q, want %q", got, test.want)
			}
		})
	}
}

func int64Pointer(value int64) *int64 { return &value }

func stringPointer(value string) *string { return &value }
