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

	if err := db.BootstrapStaff(context.Background(), seed.ShopID, "owner@atelier-demo.example", "Demo Owner", "integration-password"); err != nil {
		t.Fatalf("bootstrap seeded staff owner: %v", err)
	}
	badLogin := performAPIRequest(mux, http.MethodPost, "/v1/admin/login", "",
		`{"email":"owner@atelier-demo.example","password":"incorrect-password"}`)
	if badLogin.Code != http.StatusUnauthorized {
		t.Fatalf("invalid admin login status/body = %d/%s; want 401", badLogin.Code, badLogin.Body)
	}
	loginResponse := performAPIRequest(mux, http.MethodPost, "/v1/admin/login", "",
		`{"email":"owner@atelier-demo.example","password":"integration-password"}`)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("admin login status/body = %d/%s; want 200", loginResponse.Code, loginResponse.Body)
	}
	var staffLogin struct {
		Data struct {
			Token string `json:"token"`
			Role  string `json:"role"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&staffLogin); err != nil {
		t.Fatalf("decode staff login response: %v", err)
	}
	if staffLogin.Data.Token == "" || staffLogin.Data.Role != "owner" {
		t.Fatalf("staff login payload = %+v; want a token and owner role", staffLogin.Data)
	}
	staffAuthorization := "Bearer " + staffLogin.Data.Token
	meResponse := performAPIRequest(mux, http.MethodGet, "/v1/admin/me", staffAuthorization, "")
	if meResponse.Code != http.StatusOK || !strings.Contains(meResponse.Body.String(), `"role":"owner"`) {
		t.Fatalf("admin identity status/body = %d/%s; want owner identity", meResponse.Code, meResponse.Body)
	}

	adminCategory := performAPIRequest(mux, http.MethodPost, "/v1/admin/categories", staffAuthorization,
		`{"slug":"integration-accessoires","translations":[{"locale":"fr-FR","name":"Accessoires"}]}`)
	if adminCategory.Code != http.StatusCreated {
		t.Fatalf("admin category create status/body = %d/%s; want 201", adminCategory.Code, adminCategory.Body)
	}
	var createdCategory struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(adminCategory.Body).Decode(&createdCategory); err != nil {
		t.Fatalf("decode admin category: %v", err)
	}
	if createdCategory.Data.ID == "" {
		t.Fatal("admin category response has no ID")
	}

	productBody := `{"slug":"integration-article","status":"active","translations":[{"locale":"fr-FR","name":"Article d’intégration"}],"category_ids":["` +
		createdCategory.Data.ID + `"],"variants":[{"sku":"INTEGRATION-ARTICLE-1","options":{"taille":"unique"},"prices":[{"currency":"EUR","amount":"12.5000"}],"inventory":{"quantity":8,"track":true}}]}`
	adminProduct := performAPIRequest(mux, http.MethodPost, "/v1/admin/products", staffAuthorization, productBody)
	if adminProduct.Code != http.StatusCreated {
		t.Fatalf("admin product create status/body = %d/%s; want 201", adminProduct.Code, adminProduct.Body)
	}
	var createdProduct struct {
		Data struct {
			ID          string   `json:"id"`
			CategoryIDs []string `json:"category_ids"`
			Variants    []struct {
				ID     string `json:"id"`
				Prices []struct {
					Amount string `json:"amount"`
				} `json:"prices"`
				Inventory struct {
					Quantity int `json:"quantity"`
				} `json:"inventory"`
			} `json:"variants"`
		} `json:"data"`
	}
	if err := json.NewDecoder(adminProduct.Body).Decode(&createdProduct); err != nil {
		t.Fatalf("decode admin product: %v", err)
	}
	if createdProduct.Data.ID == "" || len(createdProduct.Data.CategoryIDs) != 1 ||
		createdProduct.Data.CategoryIDs[0] != createdCategory.Data.ID || len(createdProduct.Data.Variants) != 1 ||
		createdProduct.Data.Variants[0].Prices[0].Amount != "12.5000" ||
		createdProduct.Data.Variants[0].Inventory.Quantity != 8 {
		t.Fatalf("created admin product payload = %+v", createdProduct.Data)
	}
	updatedProductBody := `{"slug":"integration-article","status":"active","translations":[{"locale":"fr-FR","name":"Article mis à jour"}],"category_ids":["` +
		createdCategory.Data.ID + `"],"variants":[{"sku":"INTEGRATION-ARTICLE-1","options":{"taille":"unique"},"prices":[{"currency":"EUR","amount":"15.0000"}],"inventory":{"quantity":6,"track":true}}]}`
	updatedProduct := performAPIRequest(mux, http.MethodPut, "/v1/admin/products/"+createdProduct.Data.ID, staffAuthorization, updatedProductBody)
	if updatedProduct.Code != http.StatusOK ||
		!strings.Contains(updatedProduct.Body.String(), `"amount":"15.0000"`) ||
		!strings.Contains(updatedProduct.Body.String(), `"quantity":6`) {
		t.Fatalf("admin product update status/body = %d/%s; want saved price and inventory", updatedProduct.Code, updatedProduct.Body)
	}
	adminProductList := performAPIRequest(mux, http.MethodGet, "/v1/admin/products", staffAuthorization, "")
	if adminProductList.Code != http.StatusOK || !strings.Contains(adminProductList.Body.String(), createdProduct.Data.ID) {
		t.Fatalf("admin product list status/body = %d/%s; want created product", adminProductList.Code, adminProductList.Body)
	}
	archiveProduct := performAPIRequest(mux, http.MethodDelete, "/v1/admin/products/"+createdProduct.Data.ID, staffAuthorization, "")
	if archiveProduct.Code != http.StatusNoContent {
		t.Fatalf("admin product archive status/body = %d/%s; want 204", archiveProduct.Code, archiveProduct.Body)
	}
	deleteCategory := performAPIRequest(mux, http.MethodDelete, "/v1/admin/categories/"+createdCategory.Data.ID, staffAuthorization, "")
	if deleteCategory.Code != http.StatusNoContent {
		t.Fatalf("admin category delete status/body = %d/%s; want 204", deleteCategory.Code, deleteCategory.Body)
	}
	logoutResponse := performAPIRequest(mux, http.MethodPost, "/v1/admin/logout", staffAuthorization, "")
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("admin logout status/body = %d/%s; want 204", logoutResponse.Code, logoutResponse.Body)
	}
	rejectedAfterLogout := performAPIRequest(mux, http.MethodGet, "/v1/admin/me", staffAuthorization, "")
	if rejectedAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("admin request after logout status/body = %d/%s; want 401", rejectedAfterLogout.Code, rejectedAfterLogout.Body)
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
