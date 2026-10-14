package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewHandlerWiresMemoryCatalogAndStorefront(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	handler := newHandler()

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
