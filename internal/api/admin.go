package api

import (
	"context"
	"encoding/json"
	"errors"
	"euromni/internal/seed"
	"euromni/internal/store"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type StaffAuthenticator interface {
	AuthenticateStaff(ctx context.Context, shopID, email, password string) (*store.StaffSession, error)
	ValidateStaffSession(ctx context.Context, token string) (*store.StaffSession, error)
	RevokeStaffSession(ctx context.Context, token string) error
}

type contextKey string

const staffSessionContextKey contextKey = "staff-session"

var staffAuthenticator StaffAuthenticator = newDefaultStaffAuthenticator()

func SetStaffAuthenticator(auth StaffAuthenticator) {
	if auth == nil {
		staffAuthenticator = newDefaultStaffAuthenticator()
		return
	}
	staffAuthenticator = auth
}

func currentStaffAuthenticator() StaffAuthenticator {
	if staffAuthenticator == nil {
		staffAuthenticator = newDefaultStaffAuthenticator()
	}
	return staffAuthenticator
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	ShopID   string `json:"shop_id"`
}

type staffSessionPayload struct {
	Token     string `json:"token"`
	ShopID    string `json:"shop_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	ExpiresAt string `json:"expires_at"`
}

type defaultStaffAuthenticator struct{}

func newDefaultStaffAuthenticator() *defaultStaffAuthenticator {
	return &defaultStaffAuthenticator{}
}

func (a *defaultStaffAuthenticator) AuthenticateStaff(ctx context.Context, shopID, email, password string) (*store.StaffSession, error) {
	db, ok := currentProductReader().(*store.Store)
	if !ok || db.SQL == nil {
		return nil, store.ErrAuthUnavailable
	}
	return db.AuthenticateStaff(ctx, shopID, email, password)
}

func (a *defaultStaffAuthenticator) ValidateStaffSession(ctx context.Context, token string) (*store.StaffSession, error) {
	db, ok := currentProductReader().(*store.Store)
	if !ok || db.SQL == nil {
		return nil, store.ErrAuthUnavailable
	}
	return db.ValidateStaffSession(ctx, token)
}

func (a *defaultStaffAuthenticator) RevokeStaffSession(ctx context.Context, token string) error {
	db, ok := currentProductReader().(*store.Store)
	if !ok || db.SQL == nil {
		return store.ErrAuthUnavailable
	}
	return db.RevokeStaffSession(ctx, token)
}

func adminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writePrivateJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid login payload"})
		return
	}
	if strings.TrimSpace(req.Email) == "" || strings.TrimSpace(req.Password) == "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "email and password are required"})
		return
	}
	shopID := strings.TrimSpace(req.ShopID)
	if shopID == "" {
		shopID = seed.ShopID
	}

	session, err := currentStaffAuthenticator().AuthenticateStaff(r.Context(), shopID, req.Email, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrAuthUnavailable) {
			writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "staff authentication is unavailable"})
			return
		}
		if errors.Is(err, store.ErrInvalidCredentials) {
			writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
			return
		}
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}

	writePrivateJSON(w, http.StatusOK, map[string]any{"data": staffSessionPayload{
		Token:     session.Token,
		ShopID:    session.ShopID,
		UserID:    session.UserID,
		Name:      session.Name,
		Email:     session.Email,
		Role:      session.Role,
		ExpiresAt: session.ExpiresAt.UTC().Format(time.RFC3339),
	}})
}

func adminMe(w http.ResponseWriter, r *http.Request) {
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"shop_id": session.ShopID,
		"user_id": session.UserID,
		"email":   session.Email,
		"name":    session.Name,
		"role":    session.Role,
	}})
}

func adminLogout(w http.ResponseWriter, r *http.Request) {
	token := staffTokenFromRequest(r)
	if token == "" {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	if err := currentStaffAuthenticator().RevokeStaffSession(r.Context(), token); err != nil {
		if errors.Is(err, store.ErrAuthUnavailable) {
			writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "staff authentication is unavailable"})
			return
		}
		if errors.Is(err, store.ErrInvalidCredentials) {
			writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
			return
		}
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func adminProducts(w http.ResponseWriter, r *http.Request) {
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	writer, ok := currentProductReader().(store.CatalogAdminWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "catalog administration unavailable"})
		return
	}
	products, err := writer.ListAdminProducts(r.Context(), session.ShopID)
	if err != nil {
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	if products == nil {
		products = []store.ProductView{}
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": products})
}

func adminCategories(w http.ResponseWriter, r *http.Request) {
	session, ok := adminSessionFromContext(r.Context())
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
		return
	}
	reader, ok := currentProductReader().(store.CategoryReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "category reader unavailable"})
		return
	}
	categories, err := reader.ListCategories(r.Context(), session.ShopID)
	if err != nil {
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": categories})
}

func withStaffAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := staffSessionFromRequest(r)
		if !ok {
			writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "staff session required"})
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), staffSessionContextKey, session))
		next(w, r)
	}
}

func withCatalogWriteAuth(next http.HandlerFunc) http.HandlerFunc {
	return withStaffAuth(func(w http.ResponseWriter, r *http.Request) {
		session, ok := adminSessionFromContext(r.Context())
		if !ok || (session.Role != "owner" && session.Role != "admin" && session.Role != "editor") {
			writePrivateJSON(w, http.StatusForbidden, map[string]any{"error": "catalog write permission required"})
			return
		}
		next(w, r)
	})
}

var catalogSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var catalogLocalePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)
var catalogSKU = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var catalogCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
var catalogAmount = regexp.MustCompile(`^(?:0|[1-9][0-9]{0,14})(?:\.[0-9]{1,4})?$`)

func adminCreateProduct(w http.ResponseWriter, r *http.Request) {
	var input store.ProductInput
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	if err := validateProductInput(input); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.CatalogAdminWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "catalog administration unavailable"})
		return
	}
	product, err := writer.CreateProduct(r.Context(), session.ShopID, input)
	if err != nil {
		writeCatalogWriteError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusCreated, map[string]any{"data": product})
}

func adminUpdateProduct(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "product id is required"})
		return
	}
	var input store.ProductInput
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	if err := validateProductInput(input); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.CatalogAdminWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "catalog administration unavailable"})
		return
	}
	product, err := writer.UpdateProduct(r.Context(), session.ShopID, id, input)
	if err != nil {
		writeCatalogWriteError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": product})
}

func adminArchiveProduct(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "product id is required"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.CatalogAdminWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "catalog administration unavailable"})
		return
	}
	if err := writer.ArchiveProduct(r.Context(), session.ShopID, id); err != nil {
		writeCatalogWriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func adminCreateCategory(w http.ResponseWriter, r *http.Request) {
	var input store.CategoryInput
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	if err := validateCategoryInput(input); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.CatalogAdminWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "catalog administration unavailable"})
		return
	}
	category, err := writer.CreateCategory(r.Context(), session.ShopID, input)
	if err != nil {
		writeCatalogWriteError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusCreated, map[string]any{"data": category})
}

func adminUpdateCategory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "category id is required"})
		return
	}
	var input store.CategoryInput
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	if err := validateCategoryInput(input); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.CatalogAdminWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "catalog administration unavailable"})
		return
	}
	category, err := writer.UpdateCategory(r.Context(), session.ShopID, id, input)
	if err != nil {
		writeCatalogWriteError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": category})
}

func adminDeleteCategory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "category id is required"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.CatalogAdminWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "catalog administration unavailable"})
		return
	}
	if err := writer.DeleteCategory(r.Context(), session.ShopID, id); err != nil {
		writeCatalogWriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeAdminJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateProductInput(input store.ProductInput) error {
	if !catalogSlugPattern.MatchString(input.Slug) || len(input.Slug) > 128 {
		return errors.New("slug must contain lowercase letters, numbers, and single hyphens")
	}
	switch input.Status {
	case "draft", "active", "archived":
	default:
		return errors.New("status must be draft, active, or archived")
	}
	if len(input.Translations) == 0 || len(input.Translations) > 32 {
		return errors.New("at least one and at most 32 translations are required")
	}
	seen := make(map[string]struct{}, len(input.Translations))
	for _, translation := range input.Translations {
		if !validTranslation(translation.Locale, translation.Name, translation.Description) {
			return errors.New("each translation requires a valid locale and name")
		}
		locale := strings.ToLower(translation.Locale)
		if _, exists := seen[locale]; exists {
			return errors.New("translation locales must be unique")
		}
		seen[locale] = struct{}{}
	}
	if len(input.Variants) > 100 {
		return errors.New("at most 100 variants are allowed")
	}
	if input.Status == "active" && len(input.Variants) == 0 {
		return errors.New("active products require at least one variant")
	}
	variantSKUs := make(map[string]struct{}, len(input.Variants))
	for i := range input.Variants {
		if input.Variants[i].Options == nil {
			input.Variants[i].Options = map[string]any{}
		}
		variant := input.Variants[i]
		if !catalogSKU.MatchString(variant.SKU) {
			return errors.New("each variant requires a valid SKU")
		}
		sku := strings.ToUpper(variant.SKU)
		if _, exists := variantSKUs[sku]; exists {
			return errors.New("variant SKUs must be unique")
		}
		variantSKUs[sku] = struct{}{}
		if variant.Inventory.Quantity < 0 {
			return errors.New("inventory quantity cannot be negative")
		}
		if len(variant.Prices) > 10 {
			return errors.New("at most 10 prices are allowed per variant")
		}
		if input.Status == "active" && len(variant.Prices) == 0 {
			return errors.New("active product variants require at least one price")
		}
		currencies := make(map[string]struct{}, len(variant.Prices))
		for _, price := range variant.Prices {
			if !catalogCurrency.MatchString(price.Currency) || !catalogAmount.MatchString(price.Amount) {
				return errors.New("prices require an uppercase currency code and a non-negative amount with at most four decimals")
			}
			if price.CompareAt != nil && !catalogAmount.MatchString(*price.CompareAt) {
				return errors.New("compare_at must be a non-negative amount with at most four decimals")
			}
			if _, exists := currencies[price.Currency]; exists {
				return errors.New("price currencies must be unique per variant")
			}
			currencies[price.Currency] = struct{}{}
		}
		options, err := json.Marshal(variant.Options)
		if err != nil || len(options) > 16<<10 {
			return errors.New("variant options must be a JSON object smaller than 16 KiB")
		}
	}
	if len(input.CategoryIDs) > 100 {
		return errors.New("at most 100 product categories are allowed")
	}
	categories := make(map[string]struct{}, len(input.CategoryIDs))
	for _, categoryID := range input.CategoryIDs {
		if strings.TrimSpace(categoryID) == "" {
			return errors.New("category ids must not be empty")
		}
		if _, exists := categories[categoryID]; exists {
			return errors.New("category ids must be unique")
		}
		categories[categoryID] = struct{}{}
	}
	return nil
}

func validateCategoryInput(input store.CategoryInput) error {
	if !catalogSlugPattern.MatchString(input.Slug) || len(input.Slug) > 128 {
		return errors.New("slug must contain lowercase letters, numbers, and single hyphens")
	}
	if len(input.Translations) == 0 || len(input.Translations) > 32 {
		return errors.New("at least one and at most 32 translations are required")
	}
	seen := make(map[string]struct{}, len(input.Translations))
	for _, translation := range input.Translations {
		if !validTranslation(translation.Locale, translation.Name, translation.Description) {
			return errors.New("each translation requires a valid locale and name")
		}
		locale := strings.ToLower(translation.Locale)
		if _, exists := seen[locale]; exists {
			return errors.New("translation locales must be unique")
		}
		seen[locale] = struct{}{}
	}
	return nil
}

func validTranslation(locale, name, description string) bool {
	if len(locale) > 35 || !catalogLocalePattern.MatchString(locale) || len(name) == 0 || len(name) > 200 || len(description) > 10000 {
		return false
	}
	return strings.TrimSpace(name) != ""
}

func writeAdminInputError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body too large"})
		return
	}
	writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON payload"})
}

func writeCatalogWriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writePrivateJSON(w, http.StatusNotFound, map[string]any{"error": "catalog item not found"})
	case errors.Is(err, store.ErrConflict):
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "catalog item conflicts with existing data"})
	default:
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func staffSessionFromRequest(r *http.Request) (*store.StaffSession, bool) {
	if token := staffTokenFromRequest(r); token != "" {
		session, err := currentStaffAuthenticator().ValidateStaffSession(r.Context(), token)
		if err == nil {
			return session, true
		}
	}
	return nil, false
}

func staffTokenFromRequest(r *http.Request) string {
	if token := firstBearerToken(r); token != "" {
		return token
	}
	if cookie, err := r.Cookie("euromni_staff_session"); err == nil {
		return cookie.Value
	}
	return ""
}

func firstBearerToken(r *http.Request) string {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		return ""
	}
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func adminSessionFromContext(ctx context.Context) (*store.StaffSession, bool) {
	session, ok := ctx.Value(staffSessionContextKey).(*store.StaffSession)
	if !ok || session == nil {
		return nil, false
	}
	return session, true
}
