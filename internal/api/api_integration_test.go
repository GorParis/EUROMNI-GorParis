//go:build integration

package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/store"
	"euromni/internal/testdb"
)

func TestPostgresAPIEndToEnd(t *testing.T) {
	db := store.NewSQL(testdb.Start(t))
	originalReader := productReader
	SetProductReader(db)
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)
	listResponse := performAPIRequest(mux, http.MethodGet, "/v1/products", "", "")
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"slug":"cabas-lin"`) || !strings.Contains(listResponse.Body.String(), `"slug":"ceinture-cuir"`) {
		t.Fatalf("REST catalog list status/body = %d/%s", listResponse.Code, listResponse.Body)
	}
	productResponse := performAPIRequest(mux, http.MethodGet, "/v1/products/"+seed.ProductID, "", "")
	if productResponse.Code != http.StatusOK {
		t.Fatalf("REST product status = %d; body: %s", productResponse.Code, productResponse.Body)
	}

	categoryResponse := performAPIRequest(mux, http.MethodGet, "/v1/categories", "", "")
	if categoryResponse.Code != http.StatusOK || !strings.Contains(categoryResponse.Body.String(), `"slug":"sacs"`) {
		t.Fatalf("REST categories status/body = %d/%s", categoryResponse.Code, categoryResponse.Body)
	}

	createResponse := performAPIRequest(mux, http.MethodPost, "/v1/cart", "", "")
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("REST cart creation status = %d; body: %s", createResponse.Code, createResponse.Body)
	}
	var created struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode cart token: %v", err)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(created.Data.Token)
	if err != nil {
		t.Fatal("decode generated cart token failed")
	}
	tokenHash := sha256.Sum256(rawToken)
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", tokenHash[:])
	}()

	addBody := `{"variant_id":"` + seed.VariantID + `","quantity":2}`
	addResponse := performAPIRequest(mux, http.MethodPost, "/v1/cart/add", "Bearer "+created.Data.Token, addBody)
	if addResponse.Code != http.StatusOK || !strings.Contains(addResponse.Body.String(), `"amount":"178.0000"`) {
		t.Fatalf("REST cart add status/body = %d/%s", addResponse.Code, addResponse.Body)
	}

	queryBody := `{"query":"{ products { slug } product(id: \"` + seed.ProductID + `\") { slug variants { prices { amount } } } categories { slug } cart { total { amount currency } } }"}`
	graphqlResponse := performAPIRequest(mux, http.MethodPost, "/graphql", "Bearer "+created.Data.Token, queryBody)
	if graphqlResponse.Code != http.StatusOK || !strings.Contains(graphqlResponse.Body.String(), `"amount":"178.0000"`) || !strings.Contains(graphqlResponse.Body.String(), `"slug":"sacs"`) || !strings.Contains(graphqlResponse.Body.String(), `"slug":"ceinture-cuir"`) {
		t.Fatalf("GraphQL status/body = %d/%s", graphqlResponse.Code, graphqlResponse.Body)
	}

	checkoutBody := `{"email":"checkout@example.com","name":"Checkout integration","shipping_address":{"line1":"1 rue du Test","city":"Paris","postal_code":"75001","country":"FR"}}`
	checkoutResponse := performAPIRequest(mux, http.MethodPost, "/v1/checkout", "Bearer "+created.Data.Token, checkoutBody)
	if checkoutResponse.Code != http.StatusCreated || !strings.Contains(checkoutResponse.Body.String(), `"grand_total":"178.0000"`) {
		t.Fatalf("REST checkout status/body = %d/%s", checkoutResponse.Code, checkoutResponse.Body)
	}
}

func performAPIRequest(mux *http.ServeMux, method, path, authorization, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}
