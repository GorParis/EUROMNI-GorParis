package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/store"
)

func TestStorefrontServesShadowCatalog(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux)

	page := httptest.NewRecorder()
	mux.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/storefront/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("storefront status = %d, want %d", page.Code, http.StatusOK)
	}
	html := page.Body.String()
	if !strings.Contains(html, "euromni-catalog") || !strings.Contains(html, "Shadow DOM") {
		t.Fatalf("storefront page missing catalog component: %s", html)
	}

	script := httptest.NewRecorder()
	mux.ServeHTTP(script, httptest.NewRequest(http.MethodGet, "/storefront/euromni-catalog.js", nil))
	if script.Code != http.StatusOK {
		t.Fatalf("catalog script status = %d, want %d", script.Code, http.StatusOK)
	}
	js := script.Body.String()
	if !strings.Contains(js, "attachShadow") || !strings.Contains(js, "customElements.define(\"euromni-catalog\"") {
		t.Fatalf("catalog script missing shadow isolation: %s", js)
	}
	for _, feature := range []string{"cart-src", "sessionStorage", "Authorization", "Ajouter au panier", "cart-total"} {
		if !strings.Contains(js, feature) {
			t.Errorf("catalog script missing storefront cart feature %q", feature)
		}
	}
}

func TestProductsListReturnsActiveCatalog(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/v1/products", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}
	var body struct {
		Data []store.ProductView `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 5 || body.Data[0].Slug != "bougie-cedre" || body.Data[4].Slug != "tasse-gres" {
		t.Fatalf("list = %+v, want five representative products sorted by slug", body.Data)
	}
}

func TestProductByIDReturnsProductData(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/v1/products/"+seed.ProductID, nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}

	var body struct {
		Data store.ProductView `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.ID != seed.ProductID {
		t.Errorf("product id = %q, want %q", body.Data.ID, seed.ProductID)
	}
	if body.Data.Slug != "cabas-lin" {
		t.Errorf("product slug = %q, want %q", body.Data.Slug, "cabas-lin")
	}
}

func TestProductsListFiltersByCategory(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/v1/products?category="+seed.CategoryID, nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}

	var body struct {
		Data []store.ProductView `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != seed.ProductID {
		t.Fatalf("filtered products = %+v, want single product %s in the demo category", body.Data, seed.ProductID)
	}
}

func TestProductEndpointsHideStoreErrors(t *testing.T) {
	originalReader := productReader
	SetProductReader(productReaderFunc(func(context.Context, string, string) (*store.ProductView, error) {
		return nil, errors.New("postgres password=super-secret")
	}))
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	tests := []struct {
		name string
		path string
	}{
		{name: "list", path: "/v1/products"},
		{name: "by id", path: "/v1/products/" + seed.ProductID},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			Register(mux)
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusInternalServerError, response.Body)
			}
			var body map[string]string
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body["error"] != "internal server error" {
				t.Errorf("error = %q, want generic internal error", body["error"])
			}
		})
	}
}

func TestProductByIDReturnsNotFoundForMissingProduct(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)
	request := httptest.NewRequest(http.MethodGet, "/v1/products/missing-product", nil)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusNotFound, response.Body)
	}
}

func TestCategoryEndpointsReturnSeedData(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	t.Run("list", func(t *testing.T) {
		mux := http.NewServeMux()
		Register(mux)
		request := httptest.NewRequest(http.MethodGet, "/v1/categories", nil)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)

		var body struct {
			Data []struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			} `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Code != http.StatusOK || len(body.Data) != 1 || body.Data[0].ID != seed.CategoryID || body.Data[0].Slug != "sacs" {
			t.Fatalf("status = %d, categories = %+v; want seed category", response.Code, body.Data)
		}
	})

	t.Run("by id", func(t *testing.T) {
		mux := http.NewServeMux()
		Register(mux)
		request := httptest.NewRequest(http.MethodGet, "/v1/categories/"+seed.CategoryID, nil)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)

		var body struct {
			Data struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			} `json:"data"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Code != http.StatusOK || body.Data.ID != seed.CategoryID || body.Data.Slug != "sacs" {
			t.Fatalf("status = %d, category = %+v; want seed category", response.Code, body.Data)
		}
	})
}

func TestCartBearerFlow(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v1/cart", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized cart status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}
	if unauthorized.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("unauthorized cart cache-control = %q, want no-store", unauthorized.Header().Get("Cache-Control"))
	}

	createResponse := httptest.NewRecorder()
	mux.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/v1/cart", nil))
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("create cart status = %d, want %d; body: %s", createResponse.Code, http.StatusCreated, createResponse.Body)
	}
	var created struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode created cart: %v", err)
	}
	if created.Data.Token == "" {
		t.Fatal("cart creation returned an empty token")
	}

	addBody := `{"variant_id":"` + seed.VariantID + `","quantity":2}`
	addRequest := httptest.NewRequest(http.MethodPost, "/v1/cart/add", strings.NewReader(addBody))
	addRequest.Header.Set("Authorization", "Bearer "+created.Data.Token)
	addResponse := httptest.NewRecorder()
	mux.ServeHTTP(addResponse, addRequest)
	if addResponse.Code != http.StatusOK {
		t.Fatalf("add cart item status = %d, want %d; body: %s", addResponse.Code, http.StatusOK, addResponse.Body)
	}
	var added struct {
		Data store.CartView `json:"data"`
	}
	if err := json.NewDecoder(addResponse.Body).Decode(&added); err != nil {
		t.Fatalf("decode updated cart: %v", err)
	}
	if len(added.Data.Items) != 1 || added.Data.Items[0].Quantity != 2 {
		t.Fatalf("cart items = %+v, want one item with quantity 2", added.Data.Items)
	}
	if added.Data.Total.Amount != "178.0000" || added.Data.Total.Currency != "EUR" {
		t.Fatalf("cart total = %+v, want EUR 178.0000", added.Data.Total)
	}
	if addResponse.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("cart cache-control = %q, want no-store", addResponse.Header().Get("Cache-Control"))
	}

	viewRequest := httptest.NewRequest(http.MethodGet, "/v1/cart", nil)
	viewRequest.Header.Set("Authorization", "Bearer "+created.Data.Token)
	viewResponse := httptest.NewRecorder()
	mux.ServeHTTP(viewResponse, viewRequest)
	if viewResponse.Code != http.StatusOK {
		t.Fatalf("view cart status = %d, want %d; body: %s", viewResponse.Code, http.StatusOK, viewResponse.Body)
	}
}

func TestCartRejectsOversizedBodyAndToken(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})
	token, err := createCartToken(context.Background(), store.NewMemory())
	if err != nil {
		t.Fatalf("create cart token: %v", err)
	}
	mux := http.NewServeMux()
	Register(mux)

	largeBody := strings.NewReader(strings.Repeat(" ", maxCartRequestBytes+1))
	bodyRequest := httptest.NewRequest(http.MethodPost, "/v1/cart/add", largeBody)
	bodyRequest.Header.Set("Authorization", "Bearer "+token)
	bodyResponse := httptest.NewRecorder()
	mux.ServeHTTP(bodyResponse, bodyRequest)
	if bodyResponse.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body status = %d, want %d", bodyResponse.Code, http.StatusRequestEntityTooLarge)
	}

	longTokenRequest := httptest.NewRequest(http.MethodGet, "/v1/cart", nil)
	longTokenRequest.Header.Set("Authorization", "Bearer "+strings.Repeat("a", maxCartRequestBytes))
	longTokenResponse := httptest.NewRecorder()
	mux.ServeHTTP(longTokenResponse, longTokenRequest)
	if longTokenResponse.Code != http.StatusUnauthorized {
		t.Fatalf("oversized token status = %d, want %d", longTokenResponse.Code, http.StatusUnauthorized)
	}
}

func TestCheckoutCreatesOrderAndConsumesMemoryStock(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() { SetProductReader(originalReader) })
	mux := http.NewServeMux()
	Register(mux)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/v1/checkout", strings.NewReader(`{}`)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("checkout without token status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	createResponse := httptest.NewRecorder()
	mux.ServeHTTP(createResponse, httptest.NewRequest(http.MethodPost, "/v1/cart", nil))
	var created struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode cart token: %v", err)
	}
	addBody := `{"variant_id":"` + seed.VariantID + `","quantity":2}`
	addRequest := httptest.NewRequest(http.MethodPost, "/v1/cart/add", strings.NewReader(addBody))
	addRequest.Header.Set("Authorization", "Bearer "+created.Data.Token)
	addResponse := httptest.NewRecorder()
	mux.ServeHTTP(addResponse, addRequest)
	if addResponse.Code != http.StatusOK {
		t.Fatalf("add cart line status = %d, body = %s", addResponse.Code, addResponse.Body)
	}

	checkoutBody := `{"email":"buyer@example.com","name":"Client de test","shipping_address":{"line1":"1 rue du Test","city":"Paris","postal_code":"75001","country":"FR"}}`
	checkoutRequest := httptest.NewRequest(http.MethodPost, "/v1/checkout", strings.NewReader(checkoutBody))
	checkoutRequest.Header.Set("Authorization", "Bearer "+created.Data.Token)
	checkoutRequest.Header.Set("Content-Type", "application/json")
	checkoutResponse := httptest.NewRecorder()
	mux.ServeHTTP(checkoutResponse, checkoutRequest)
	if checkoutResponse.Code != http.StatusCreated {
		t.Fatalf("checkout status = %d, body = %s", checkoutResponse.Code, checkoutResponse.Body)
	}
	if checkoutResponse.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("checkout cache-control = %q, want no-store", checkoutResponse.Header().Get("Cache-Control"))
	}
	var order struct {
		Data store.OrderView `json:"data"`
	}
	if err := json.NewDecoder(checkoutResponse.Body).Decode(&order); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if order.Data.Status != "pending" || order.Data.Subtotal != "178.0000" || len(order.Data.Items) != 1 {
		t.Fatalf("order = %+v, want pending order for EUR 178.0000", order.Data)
	}

	emptyCheckout := httptest.NewRecorder()
	retry := httptest.NewRequest(http.MethodPost, "/v1/checkout", strings.NewReader(checkoutBody))
	retry.Header.Set("Authorization", "Bearer "+created.Data.Token)
	mux.ServeHTTP(emptyCheckout, retry)
	if emptyCheckout.Code != http.StatusConflict {
		t.Fatalf("checkout retry status = %d, want %d", emptyCheckout.Code, http.StatusConflict)
	}

	invalidRequest := httptest.NewRequest(http.MethodPost, "/v1/checkout", strings.NewReader(`{"email":"bad","name":"","shipping_address":{}}`))
	invalidRequest.Header.Set("Authorization", "Bearer "+created.Data.Token)
	invalidResponse := httptest.NewRecorder()
	mux.ServeHTTP(invalidResponse, invalidRequest)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid checkout details status = %d, want %d", invalidResponse.Code, http.StatusBadRequest)
	}
}

func TestGraphQLCategoryQueriesReturnSeedData(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	query := `{"query":"{ categories { id slug } category(id: \"` + seed.CategoryID + `\") { id slug } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux)
	mux.ServeHTTP(response, request)

	var body struct {
		Data struct {
			Categories []struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			} `json:"categories"`
			Category struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			} `json:"category"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Errors) != 0 {
		t.Fatalf("unexpected GraphQL errors: %+v", body.Errors)
	}
	if len(body.Data.Categories) != 1 || body.Data.Categories[0].ID != seed.CategoryID || body.Data.Categories[0].Slug != "sacs" {
		t.Errorf("categories = %+v, want seeded category", body.Data.Categories)
	}
	if body.Data.Category.ID != seed.CategoryID || body.Data.Category.Slug != "sacs" {
		t.Errorf("category = %+v, want seeded category", body.Data.Category)
	}
}

func TestGraphQLCartMutationsRequirePostAndBearer(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)
	createRequest := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"mutation { createCart { token } }"}`))
	createResponse := httptest.NewRecorder()
	mux.ServeHTTP(createResponse, createRequest)
	if createResponse.Code != http.StatusOK || createResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create cart status/cache = %d/%q; body: %s", createResponse.Code, createResponse.Header().Get("Cache-Control"), createResponse.Body)
	}
	var created struct {
		Data struct {
			CreateCart struct {
				Token string `json:"token"`
			} `json:"createCart"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode create cart response: %v", err)
	}
	if len(created.Errors) != 0 || created.Data.CreateCart.Token == "" {
		t.Fatalf("create cart response = %+v", created)
	}

	addBody := `{"query":"mutation Add($variantID: ID!, $quantity: Int!) { addCartLine(variantId: $variantID, quantity: $quantity) { items { sku quantity } total { amount currency } } }","variables":{"variantID":"` + seed.VariantID + `","quantity":2}}`
	addRequest := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(addBody))
	addRequest.Header.Set("Authorization", "Bearer "+created.Data.CreateCart.Token)
	addResponse := httptest.NewRecorder()
	mux.ServeHTTP(addResponse, addRequest)
	var added struct {
		Data struct {
			AddCartLine struct {
				Total struct {
					Amount string `json:"amount"`
				} `json:"total"`
				Items []struct {
					Quantity int `json:"quantity"`
				} `json:"items"`
			} `json:"addCartLine"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(addResponse.Body).Decode(&added); err != nil {
		t.Fatalf("decode add cart response: %v", err)
	}
	if len(added.Errors) != 0 || len(added.Data.AddCartLine.Items) != 1 || added.Data.AddCartLine.Items[0].Quantity != 2 || added.Data.AddCartLine.Total.Amount != "178.0000" {
		t.Fatalf("add cart response = %+v", added)
	}

	getValues := url.Values{"query": {"mutation { createCart { token } }"}}
	getRequest := httptest.NewRequest(http.MethodGet, "/graphql?"+getValues.Encode(), nil)
	getResponse := httptest.NewRecorder()
	mux.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET mutation status = %d, want %d; body: %s", getResponse.Code, http.StatusMethodNotAllowed, getResponse.Body)
	}
}

func TestGraphQLRejectsOversizedGETQuery(t *testing.T) {
	values := url.Values{"query": {strings.Repeat(" ", maxGraphQLRequestBytes+1)}}
	request := httptest.NewRequest(http.MethodGet, "/graphql?"+values.Encode(), nil)
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux)
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized GraphQL GET status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestGraphQLProductsQueryReturnsCatalog(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	query := `{"query":"{ products { id slug } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux)
	mux.ServeHTTP(response, request)

	var body struct {
		Data struct {
			Products []struct {
				ID   string `json:"id"`
				Slug string `json:"slug"`
			} `json:"products"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Errors) != 0 {
		t.Fatalf("unexpected GraphQL errors: %+v", body.Errors)
	}
	if len(body.Data.Products) != 5 || body.Data.Products[0].Slug != "bougie-cedre" || body.Data.Products[2].ID != seed.SecondProductID {
		t.Fatalf("products = %+v, want seeded catalog", body.Data.Products)
	}
}

func TestGraphQLProductQueryReturnsProductData(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	query := `{"query":"{ product(id: \"` + seed.ProductID + `\") { id slug status translations { locale name description } media { uri mimeType alt } variants { id sku options { name value } prices { currency amount compareAt } inventory { quantity track } } } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux)
	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body)
	}
	var body struct {
		Data struct {
			Product struct {
				ID           string `json:"id"`
				Slug         string `json:"slug"`
				Status       string `json:"status"`
				Translations []struct {
					Locale string `json:"locale"`
					Name   string `json:"name"`
				} `json:"translations"`
				Media []struct {
					MIMEType string `json:"mimeType"`
				} `json:"media"`
				Variants []struct {
					SKU     string                         `json:"sku"`
					Options []struct{ Name, Value string } `json:"options"`
					Prices  []struct {
						Amount string `json:"amount"`
					} `json:"prices"`
					Inventory struct {
						Quantity int `json:"quantity"`
					} `json:"inventory"`
				} `json:"variants"`
			} `json:"product"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Errors) != 0 {
		t.Fatalf("unexpected GraphQL errors: %+v", body.Errors)
	}
	product := body.Data.Product
	if product.ID != seed.ProductID || product.Slug != "cabas-lin" || product.Status != "active" {
		t.Errorf("product identity = %+v, want seed product", product)
	}
	if len(product.Translations) != 2 || product.Translations[0].Locale != "en-GB" {
		t.Errorf("translations = %+v, want both seeded locales", product.Translations)
	}
	if len(product.Media) != 1 || product.Media[0].MIMEType != "image/svg+xml" {
		t.Errorf("media = %+v (count %d), want seeded SVG image", product.Media, len(product.Media))
	}
	if len(product.Variants) != 1 || product.Variants[0].SKU != "CABAS-LIN-NAT" {
		t.Errorf("variants = %+v, want seeded variant", product.Variants)
		return
	}
	variant := product.Variants[0]
	if len(variant.Options) != 2 || variant.Options[0].Name != "color" || variant.Options[0].Value != "naturel" {
		t.Errorf("options = %+v, want sorted color and size options", variant.Options)
	}
	if len(variant.Prices) != 1 || variant.Prices[0].Amount != "89.0000" {
		t.Errorf("prices = %+v, want EUR 89.0000", variant.Prices)
	}
	if variant.Inventory.Quantity != 24 {
		t.Errorf("inventory quantity = %d, want 24", variant.Inventory.Quantity)
	}
}

func TestGraphQLDoesNotExposeStoreErrors(t *testing.T) {
	originalReader := productReader
	SetProductReader(productReaderFunc(func(context.Context, string, string) (*store.ProductView, error) {
		return nil, errors.New("postgres password=super-secret")
	}))
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	query := `{"query":"{ product(id: \"` + seed.ProductID + `\") { id } }"}`
	request := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux)
	mux.ServeHTTP(response, request)

	if strings.Contains(response.Body.String(), "super-secret") || strings.Contains(response.Body.String(), "password=") {
		t.Fatalf("response leaked store error: %s", response.Body)
	}
}

type productReaderFunc func(context.Context, string, string) (*store.ProductView, error)

func (f productReaderFunc) GetProduct(ctx context.Context, shopID, productID string) (*store.ProductView, error) {
	return f(ctx, shopID, productID)
}

func (f productReaderFunc) ListProducts(ctx context.Context, shopID string) ([]store.ProductView, error) {
	_, err := f(ctx, shopID, "")
	return nil, err
}
