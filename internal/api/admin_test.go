package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"euromni/internal/seed"
	"euromni/internal/store"
)

type testStaffAuthenticator struct {
	sessions map[string]*store.StaffSession
}

func (a *testStaffAuthenticator) AuthenticateStaff(_ context.Context, shopID, email, password string) (*store.StaffSession, error) {
	if email != "admin@example.com" || password != "hunter2" || shopID == "" {
		return nil, store.ErrInvalidCredentials
	}
	token := "demo-token-123"
	hash := sha256.Sum256([]byte(token))
	session := &store.StaffSession{ID: "session-1", ShopID: shopID, UserID: "user-1", Email: email, Name: "Demo Admin", Role: "admin", ExpiresAt: time.Now().Add(time.Hour), Token: token}
	a.sessions[string(hash[:])] = session
	return session, nil
}

func (a *testStaffAuthenticator) ValidateStaffSession(_ context.Context, token string) (*store.StaffSession, error) {
	if token == "" {
		return nil, store.ErrInvalidCredentials
	}
	hash := sha256.Sum256([]byte(token))
	session, ok := a.sessions[string(hash[:])]
	if !ok {
		return nil, store.ErrInvalidCredentials
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, store.ErrInvalidCredentials
	}
	return session, nil
}

func TestAdminLoginAndProtectedRoutes(t *testing.T) {
	auth := &testStaffAuthenticator{sessions: map[string]*store.StaffSession{}}
	SetStaffAuthenticator(auth)
	defer SetStaffAuthenticator(nil)

	mux := http.NewServeMux()
	Register(mux)

	payload := []byte(`{"email":"admin@example.com","password":"hunter2"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/login", bytes.NewReader(payload))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body=%s", res.Code, http.StatusOK, res.Body.String())
	}

	var loginResponse struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &loginResponse); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResponse.Data.Token == "" {
		t.Fatal("expected a token in login response")
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "/v1/admin/products", nil)
	protectedReq.Header.Set("Authorization", "Bearer "+loginResponse.Data.Token)
	protectedRes := httptest.NewRecorder()
	mux.ServeHTTP(protectedRes, protectedReq)
	if protectedRes.Code != http.StatusOK {
		t.Fatalf("protected products status = %d, want %d; body=%s", protectedRes.Code, http.StatusOK, protectedRes.Body.String())
	}

	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/v1/admin/products", nil)
	unauthorizedRes := httptest.NewRecorder()
	mux.ServeHTTP(unauthorizedRes, unauthorizedReq)
	if unauthorizedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d; body=%s", unauthorizedRes.Code, http.StatusUnauthorized, unauthorizedRes.Body.String())
	}
}

func TestAdminLoginIsUnavailableWithoutDatabaseAuthenticator(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() { SetProductReader(originalReader) })
	SetStaffAuthenticator(nil)
	t.Cleanup(func() { SetStaffAuthenticator(nil) })

	mux := http.NewServeMux()
	Register(mux)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/admin/login", strings.NewReader(`{"email":"admin@euromni.local","password":"admin123"}`))
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("login without DB status = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
}

func TestAdminLoginRejectsBadCredentials(t *testing.T) {
	auth := &testStaffAuthenticator{sessions: map[string]*store.StaffSession{}}
	SetStaffAuthenticator(auth)
	defer SetStaffAuthenticator(nil)

	mux := http.NewServeMux()
	Register(mux)

	payload := []byte(`{"email":"admin@example.com","password":"wrong"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/login", bytes.NewReader(payload))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("invalid credential status = %d, want %d; body=%s", res.Code, http.StatusUnauthorized, res.Body.String())
	}
}

func TestAdminCatalogCRUD(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() { SetProductReader(originalReader) })

	auth := newCatalogTestAuthenticator("admin", seed.ShopID)
	SetStaffAuthenticator(auth)
	t.Cleanup(func() { SetStaffAuthenticator(nil) })

	mux := http.NewServeMux()
	Register(mux)

	categoryBody := `{"slug":"accessoires","translations":[{"locale":"fr-FR","name":"Accessoires","description":"Objets utiles"}]}`
	categoryCreated := performAdminRequest(mux, http.MethodPost, "/v1/admin/categories", auth.token, categoryBody)
	if categoryCreated.Code != http.StatusCreated {
		t.Fatalf("create category status = %d, want %d; body=%s", categoryCreated.Code, http.StatusCreated, categoryCreated.Body.String())
	}
	var createdCategory struct {
		Data store.CategoryView `json:"data"`
	}
	if err := json.Unmarshal(categoryCreated.Body.Bytes(), &createdCategory); err != nil {
		t.Fatalf("decode created category: %v", err)
	}
	if createdCategory.Data.ID == "" || createdCategory.Data.Slug != "accessoires" {
		t.Fatalf("created category = %+v", createdCategory.Data)
	}
	otherShopAuth := newCatalogTestAuthenticator("admin", "another-shop")
	SetStaffAuthenticator(otherShopAuth)
	otherShopProducts := performAdminRequest(mux, http.MethodGet, "/v1/admin/products", otherShopAuth.token, "")
	if otherShopProducts.Code != http.StatusOK || !strings.Contains(otherShopProducts.Body.String(), `"data":[]`) {
		t.Fatalf("other shop admin product list status/body = %d/%s; want empty list", otherShopProducts.Code, otherShopProducts.Body.String())
	}
	SetStaffAuthenticator(auth)

	duplicateCategory := performAdminRequest(mux, http.MethodPost, "/v1/admin/categories", auth.token, categoryBody)
	if duplicateCategory.Code != http.StatusConflict {
		t.Fatalf("duplicate category status = %d, want %d; body=%s", duplicateCategory.Code, http.StatusConflict, duplicateCategory.Body.String())
	}
	updatedCategory := performAdminRequest(mux, http.MethodPut, "/v1/admin/categories/"+createdCategory.Data.ID, auth.token,
		`{"slug":"accessoires-mode","translations":[{"locale":"fr-FR","name":"Accessoires mode","description":""}]}`)
	if updatedCategory.Code != http.StatusOK {
		t.Fatalf("update category status = %d, want %d; body=%s", updatedCategory.Code, http.StatusOK, updatedCategory.Body.String())
	}
	deletedCategory := performAdminRequest(mux, http.MethodDelete, "/v1/admin/categories/"+createdCategory.Data.ID, auth.token, "")
	if deletedCategory.Code != http.StatusNoContent {
		t.Fatalf("delete category status = %d, want %d; body=%s", deletedCategory.Code, http.StatusNoContent, deletedCategory.Body.String())
	}

	productBody := `{"slug":"porte-cle-demo","status":"draft","translations":[{"locale":"fr-FR","name":"Porte-clés","description":"Prototype"}]}`
	productCreated := performAdminRequest(mux, http.MethodPost, "/v1/admin/products", auth.token, productBody)
	if productCreated.Code != http.StatusCreated {
		t.Fatalf("create product status = %d, want %d; body=%s", productCreated.Code, http.StatusCreated, productCreated.Body.String())
	}
	var createdProduct struct {
		Data store.ProductView `json:"data"`
	}
	if err := json.Unmarshal(productCreated.Body.Bytes(), &createdProduct); err != nil {
		t.Fatalf("decode created product: %v", err)
	}
	if createdProduct.Data.ID == "" || createdProduct.Data.Status != "draft" {
		t.Fatalf("created product = %+v", createdProduct.Data)
	}
	adminList := performAdminRequest(mux, http.MethodGet, "/v1/admin/products", auth.token, "")
	if adminList.Code != http.StatusOK || !strings.Contains(adminList.Body.String(), createdProduct.Data.ID) {
		t.Fatalf("admin product list status/body = %d/%s; expected draft product %s", adminList.Code, adminList.Body.String(), createdProduct.Data.ID)
	}

	updatedProduct := performAdminRequest(mux, http.MethodPut, "/v1/admin/products/"+createdProduct.Data.ID, auth.token,
		`{"slug":"porte-cle-demo","status":"active","translations":[{"locale":"fr-FR","name":"Porte-clés","description":"Mis à jour"}]}`)
	if updatedProduct.Code != http.StatusOK {
		t.Fatalf("update product status = %d, want %d; body=%s", updatedProduct.Code, http.StatusOK, updatedProduct.Body.String())
	}
	publicProduct := performAdminRequest(mux, http.MethodGet, "/v1/products/"+createdProduct.Data.ID, "", "")
	if publicProduct.Code != http.StatusOK {
		t.Fatalf("published product status = %d, want %d; body=%s", publicProduct.Code, http.StatusOK, publicProduct.Body.String())
	}
	archivedProduct := performAdminRequest(mux, http.MethodDelete, "/v1/admin/products/"+createdProduct.Data.ID, auth.token, "")
	if archivedProduct.Code != http.StatusNoContent {
		t.Fatalf("archive product status = %d, want %d; body=%s", archivedProduct.Code, http.StatusNoContent, archivedProduct.Body.String())
	}
	publicAfterArchive := performAdminRequest(mux, http.MethodGet, "/v1/products/"+createdProduct.Data.ID, "", "")
	if publicAfterArchive.Code != http.StatusNotFound {
		t.Fatalf("archived product public status = %d, want %d", publicAfterArchive.Code, http.StatusNotFound)
	}
}

func TestAdminCatalogValidationAndRoleAuthorization(t *testing.T) {
	originalReader := productReader
	SetProductReader(store.NewMemory())
	t.Cleanup(func() { SetProductReader(originalReader) })

	auth := newCatalogTestAuthenticator("admin", seed.ShopID)
	SetStaffAuthenticator(auth)
	t.Cleanup(func() { SetStaffAuthenticator(nil) })
	mux := http.NewServeMux()
	Register(mux)

	for _, test := range []struct {
		name   string
		body   string
		status int
	}{
		{name: "invalid slug", body: `{"slug":"Invalid Slug","status":"draft","translations":[{"locale":"fr-FR","name":"Nom"}]}`, status: http.StatusBadRequest},
		{name: "duplicate locale", body: `{"slug":"valid-slug","status":"draft","translations":[{"locale":"fr-FR","name":"Nom"},{"locale":"fr-FR","name":"Name"}]}`, status: http.StatusBadRequest},
		{name: "unknown field", body: `{"slug":"valid-slug","status":"draft","extra":true,"translations":[{"locale":"fr-FR","name":"Nom"}]}`, status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := performAdminRequest(mux, http.MethodPost, "/v1/admin/products", auth.token, test.body)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}

	validBody := `{"slug":"porte-cle-demo","status":"draft","translations":[{"locale":"fr-FR","name":"Porte-clés"}]}`
	support := newCatalogTestAuthenticator("support", seed.ShopID)
	SetStaffAuthenticator(support)
	forbidden := performAdminRequest(mux, http.MethodPost, "/v1/admin/products", support.token, validBody)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("support write status = %d, want %d; body=%s", forbidden.Code, http.StatusForbidden, forbidden.Body.String())
	}
	readAllowed := performAdminRequest(mux, http.MethodGet, "/v1/admin/products", support.token, "")
	if readAllowed.Code != http.StatusOK {
		t.Fatalf("support read status = %d, want %d; body=%s", readAllowed.Code, http.StatusOK, readAllowed.Body.String())
	}
}

type catalogTestAuth struct {
	*testStaffAuthenticator
	token string
}

func newCatalogTestAuthenticator(role, shopID string) *catalogTestAuth {
	token := "catalog-test-" + role
	hash := sha256.Sum256([]byte(token))
	auth := &testStaffAuthenticator{sessions: map[string]*store.StaffSession{}}
	auth.sessions[string(hash[:])] = &store.StaffSession{
		ID: "session-" + role, ShopID: shopID, UserID: "user-" + role, Email: role + "@example.com",
		Role: role, ExpiresAt: time.Now().Add(time.Hour), Token: token,
	}
	return &catalogTestAuth{testStaffAuthenticator: auth, token: token}
}

func performAdminRequest(mux *http.ServeMux, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}
