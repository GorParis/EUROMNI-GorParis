package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"euromni/internal/api"
	"euromni/internal/store"
)

func TestNewHandlerWiresMemoryCatalogAndStorefront(t *testing.T) {
	handler := newHandler(store.NewMemory())

	productResponse := httptest.NewRecorder()
	handler.ServeHTTP(productResponse, httptest.NewRequest(http.MethodGet, "/v1/products", nil))
	if productResponse.Code != http.StatusOK {
		t.Fatalf("product endpoint status = %d, want %d; body: %s", productResponse.Code, http.StatusOK, productResponse.Body)
	}
	if body := productResponse.Body.String(); !strings.Contains(body, `"slug":"cabas-lin"`) {
		t.Fatalf("product endpoint response does not contain the seeded catalog: %s", body)
	}

	storefrontResponse := httptest.NewRecorder()
	handler.ServeHTTP(storefrontResponse, httptest.NewRequest(http.MethodGet, "/storefront/", nil))
	if storefrontResponse.Code != http.StatusOK {
		t.Fatalf("storefront status = %d, want %d; body: %s", storefrontResponse.Code, http.StatusOK, storefrontResponse.Body)
	}
	if body := storefrontResponse.Body.String(); !strings.Contains(body, "euromni-catalog") {
		t.Fatalf("storefront response does not contain the catalog component: %s", body)
	}
}

func TestConfiguredProductReaderRejectsUnavailableDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")
	reader, closeReader, err := configuredProductReader()
	if err == nil {
		if closeReader != nil {
			_ = closeReader()
		}
		t.Fatalf("configuredProductReader() = %T, nil error; want database connection error", reader)
	}
	if reader != nil || closeReader != nil {
		t.Fatalf("configuredProductReader() returned reader=%T and close=%v on error", reader, closeReader != nil)
	}
}

func TestListenAddressDefaultsToLoopbackAndAcceptsOverride(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("PORT", "")
	if got := listenAddress(); got != "127.0.0.1:8081" {
		t.Fatalf("default listen address = %q, want 127.0.0.1:8081", got)
	}
	t.Setenv("PORT", "9090")
	if got := listenAddress(); got != "127.0.0.1:9090" {
		t.Fatalf("port-based listen address = %q, want 127.0.0.1:9090", got)
	}
	t.Setenv("LISTEN_ADDR", "0.0.0.0:8081")
	if got := listenAddress(); got != "0.0.0.0:8081" {
		t.Fatalf("explicit listen address = %q, want 0.0.0.0:8081", got)
	}
}

func TestDevelopmentPaymentProviderRequiresExplicitDevelopmentEnvironment(t *testing.T) {
	t.Cleanup(func() { api.SetDevelopmentPaymentProcessor(nil) })
	for _, test := range []struct {
		name        string
		environment string
		provider    string
		wantError   bool
	}{
		{name: "disabled by default", environment: "production"},
		{name: "enabled in development", environment: "development", provider: "development-fake"},
		{name: "blocked outside development", environment: "production", provider: "development-fake", wantError: true},
		{name: "provider validation", environment: "development", provider: "real", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("EUROMNI_ENVIRONMENT", test.environment)
			t.Setenv("EUROMNI_PAYMENT_PROVIDER", test.provider)
			err := configureDevelopmentPaymentProvider()
			if (err != nil) != test.wantError {
				t.Fatalf("configureDevelopmentPaymentProvider() error = %v, want error %t", err, test.wantError)
			}
		})
	}
}
