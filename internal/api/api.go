package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"euromni/frontend"
	"euromni/internal/market"
	"euromni/internal/seed"
	"euromni/internal/store"
)

var productReader store.ProductReader = store.ProductReaderFromEnv()
var newProductsWidgetRunner interface {
	RunNewProducts(context.Context, string) ([]store.ProductView, bool, error)
}

func SetProductReader(r store.ProductReader) {
	productReader = r
}

func SetNewProductsWidgetRunner(runner interface {
	RunNewProducts(context.Context, string) ([]store.ProductView, bool, error)
}) {
	newProductsWidgetRunner = runner
}

func currentProductReader() store.ProductReader {
	if productReader == nil {
		productReader = store.ProductReaderFromEnv()
	}
	return productReader
}

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /v1/status", status)
	mux.HandleFunc("GET /v1/reference/eu-countries", euCountriesReference)
	mux.HandleFunc("GET /v1/products", productsList)
	mux.HandleFunc("GET /v1/products/{id}", productByID)
	mux.HandleFunc("GET /v1/widgets/new-products", newProductsWidget)
	mux.HandleFunc("GET /v1/categories", categoriesList)
	mux.HandleFunc("GET /v1/categories/{id}", categoryByID)
	mux.HandleFunc("GET /v1/cart", cartView)
	mux.HandleFunc("POST /v1/cart", cartCreate)
	mux.HandleFunc("POST /v1/cart/add", cartAdd)
	mux.HandleFunc("GET /v1/shipping-options", shippingOptions)
	mux.HandleFunc("POST /v1/shipping-quotes", shippingQuote)
	mux.HandleFunc("POST /v1/checkout", checkout)
	mux.HandleFunc("POST /v1/orders/{id}/payments/development", developmentPayment)
	mux.HandleFunc("POST /v1/admin/login", adminLogin)
	mux.HandleFunc("POST /v1/admin/logout", withStaffAuth(adminLogout))
	mux.HandleFunc("GET /v1/admin/me", withStaffAuth(adminMe))
	mux.HandleFunc("GET /v1/admin/products", withStaffAuth(adminProducts))
	mux.HandleFunc("POST /v1/admin/products", withCatalogWriteAuth(adminCreateProduct))
	mux.HandleFunc("PUT /v1/admin/products/{id}", withCatalogWriteAuth(adminUpdateProduct))
	mux.HandleFunc("DELETE /v1/admin/products/{id}", withCatalogWriteAuth(adminArchiveProduct))
	mux.HandleFunc("GET /v1/admin/brands", withStaffAuth(adminBrands))
	mux.HandleFunc("POST /v1/admin/brands", withCatalogWriteAuth(adminCreateBrand))
	mux.HandleFunc("PUT /v1/admin/brands/{id}", withCatalogWriteAuth(adminUpdateBrand))
	mux.HandleFunc("GET /v1/admin/suppliers", withStaffAuth(adminSuppliers))
	mux.HandleFunc("POST /v1/admin/suppliers", withCatalogWriteAuth(adminCreateSupplier))
	mux.HandleFunc("PUT /v1/admin/suppliers/{id}", withCatalogWriteAuth(adminUpdateSupplier))
	mux.HandleFunc("GET /v1/admin/categories", withStaffAuth(adminCategories))
	mux.HandleFunc("POST /v1/admin/categories", withCatalogWriteAuth(adminCreateCategory))
	mux.HandleFunc("PUT /v1/admin/categories/{id}", withCatalogWriteAuth(adminUpdateCategory))
	mux.HandleFunc("DELETE /v1/admin/categories/{id}", withCatalogWriteAuth(adminDeleteCategory))
	mux.HandleFunc("GET /v1/admin/staff", withOwnerAuth(adminStaffList))
	mux.HandleFunc("POST /v1/admin/staff", withOwnerAuth(adminStaffCreate))
	mux.HandleFunc("PUT /v1/admin/staff/{id}", withOwnerAuth(adminStaffUpdate))
	mux.HandleFunc("DELETE /v1/admin/staff/{id}", withOwnerAuth(adminStaffRemove))
	mux.HandleFunc("GET /v1/admin/orders", withOrderReadAuth(adminOrders))
	mux.HandleFunc("GET /v1/admin/orders/{id}", withOrderReadAuth(adminOrderDetail))
	mux.HandleFunc("POST /v1/admin/orders/{id}/payments/manual", withOrderWriteAuth(adminOrderManualPayment))
	mux.HandleFunc("POST /v1/admin/orders/{id}/supplier-sync", withOrderWriteAuth(adminOrderSupplierSync))
	mux.HandleFunc("POST /v1/admin/orders/{id}/fulfill", withOrderWriteAuth(adminOrderFulfill))
	mux.HandleFunc("PATCH /v1/admin/orders/{id}/status", withOrderWriteAuth(adminOrderStatus))
	mux.HandleFunc("GET /v1/admin/shipping", withStaffAuth(adminShippingConfig))
	mux.HandleFunc("PUT /v1/admin/shipping", withOrderWriteAuth(adminShippingConfig))
	mux.HandleFunc("GET /v1/admin/modules/{name}/config", withOrderWriteAuth(adminNewProductsSettings))
	mux.HandleFunc("PUT /v1/admin/modules/{name}/config", withOrderWriteAuth(adminNewProductsSettings))
	mux.HandleFunc("GET /v1/admin/shipping/versions/targets", withOrderWriteAuth(adminShippingRateVersionTargets))
	mux.HandleFunc("GET /v1/admin/shipping/versions", withOrderWriteAuth(adminShippingRateVersions))
	mux.HandleFunc("POST /v1/admin/shipping/versions/{rateVersion}/restore", withOrderWriteAuth(adminRestoreShippingRateVersion))
	mux.HandleFunc("POST /graphql", graphql)
	mux.HandleFunc("GET /graphql", graphql)
	mux.Handle("GET /storefront/", http.StripPrefix("/storefront/", http.FileServer(http.FS(frontend.Assets))))
	adminFS, err := fs.Sub(frontend.AdminAssets, "admin")
	if err != nil {
		panic(err)
	}
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", http.FileServer(http.FS(adminFS))))
}

func newProductsWidget(w http.ResponseWriter, r *http.Request) {
	if newProductsWidgetRunner == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "new-products widget is unavailable"})
		return
	}
	products, installed, err := newProductsWidgetRunner.RunNewProducts(r.Context(), seed.ShopID)
	if err != nil {
		log.Printf("new-products widget: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "new-products widget is unavailable"})
		return
	}
	if !installed || products == nil {
		products = []store.ProductView{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": products})
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":      "euromni",
		"layers":    []string{"pipeline", "cms-go", "storefront"},
		"runtime":   "wasmtime",
		"api":       []string{"rest", "graphql"},
		"abi":       "euromni.abi.v1",
		"datastore": "postgresql",
	})
}

func euCountriesReference(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"default_locale": "fr-FR",
			"countries":      market.EUCountries(),
		},
	})
}

func productsList(w http.ResponseWriter, r *http.Request) {
	categoryID := strings.TrimSpace(r.URL.Query().Get("category"))
	reader := currentProductReader()

	var products []store.ProductView
	var err error
	if categoryID != "" {
		categoryReader, ok := reader.(store.ProductCategoryReader)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "category filtering is unavailable"})
			return
		}
		products, err = categoryReader.ListProductsByCategory(r.Context(), seed.ShopID, categoryID)
	} else {
		products, err = reader.ListProducts(r.Context(), seed.ShopID)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	if products == nil {
		products = []store.ProductView{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": products})
}

func productByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing product id"})
		return
	}

	p, err := currentProductReader().GetProduct(r.Context(), seed.ShopID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{
				"error": "product not found",
				"id":    id,
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": p})
}

func categoriesList(w http.ResponseWriter, r *http.Request) {
	reader, ok := currentProductReader().(store.CategoryReader)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	categories, err := reader.ListCategories(r.Context(), seed.ShopID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": categories})
}

func categoryByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing category id"})
		return
	}

	reader, ok := currentProductReader().(store.CategoryReader)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	category, err := reader.GetCategory(r.Context(), seed.ShopID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "category not found", "id": id})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": category})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
