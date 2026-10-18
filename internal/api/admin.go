package api

import (
	"context"
	"encoding/json"
	"errors"
	"euromni/internal/seed"
	"euromni/internal/store"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
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

func withOwnerAuth(next http.HandlerFunc) http.HandlerFunc {
	return withStaffAuth(func(w http.ResponseWriter, r *http.Request) {
		session, ok := adminSessionFromContext(r.Context())
		if !ok || session.Role != "owner" {
			writePrivateJSON(w, http.StatusForbidden, map[string]any{"error": "owner permission required"})
			return
		}
		next(w, r)
	})
}

func withOrderReadAuth(next http.HandlerFunc) http.HandlerFunc {
	return withStaffAuth(func(w http.ResponseWriter, r *http.Request) {
		session, ok := adminSessionFromContext(r.Context())
		if !ok || (session.Role != "owner" && session.Role != "admin" && session.Role != "support") {
			writePrivateJSON(w, http.StatusForbidden, map[string]any{"error": "order read permission required"})
			return
		}
		next(w, r)
	})
}

func withOrderWriteAuth(next http.HandlerFunc) http.HandlerFunc {
	return withStaffAuth(func(w http.ResponseWriter, r *http.Request) {
		session, ok := adminSessionFromContext(r.Context())
		if !ok || (session.Role != "owner" && session.Role != "admin") {
			writePrivateJSON(w, http.StatusForbidden, map[string]any{"error": "order write permission required"})
			return
		}
		next(w, r)
	})
}

func adminOrders(w http.ResponseWriter, r *http.Request) {
	session, _ := adminSessionFromContext(r.Context())
	reader, ok := currentProductReader().(store.OrderAdminReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "order administration unavailable"})
		return
	}
	limit := 50
	offset := 0
	var err error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100 {
			writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "limit must be between 1 and 100"})
			return
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil || offset < 0 || offset > 100000 {
			writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "offset must be between 0 and 100000"})
			return
		}
	}
	orders, err := reader.ListAdminOrders(r.Context(), session.ShopID, limit, offset)
	if err != nil {
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	if orders == nil {
		orders = []store.AdminOrderView{}
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": orders})
}

func adminOrderDetail(w http.ResponseWriter, r *http.Request) {
	session, _ := adminSessionFromContext(r.Context())
	reader, ok := currentProductReader().(store.OrderAdminReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "order administration unavailable"})
		return
	}
	order, err := reader.GetAdminOrder(r.Context(), session.ShopID, r.PathValue("id"))
	if err != nil {
		writeOrderAdminError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": order})
}

func adminOrderStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status string `json:"status"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	if input.Status != "cancelled" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "only cancellation is currently supported"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.OrderAdminReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "order administration unavailable"})
		return
	}
	if err := writer.CancelAdminOrder(r.Context(), session.ShopID, r.PathValue("id")); err != nil {
		writeOrderAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func adminOrderManualPayment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ProviderReference string `json:"provider_reference"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	input.ProviderReference = strings.TrimSpace(input.ProviderReference)
	if len(input.ProviderReference) > 512 {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "payment reference must be at most 512 bytes"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.OrderAdminReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "order administration unavailable"})
		return
	}
	if err := writer.ConfirmManualOrderPayment(r.Context(), session.ShopID, r.PathValue("id"), input.ProviderReference); err != nil {
		writeOrderAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func adminOrderFulfill(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Tracking string `json:"tracking"`
	}
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	input.Tracking = strings.TrimSpace(input.Tracking)
	if len(input.Tracking) > 512 {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "tracking reference must be at most 512 bytes"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	writer, ok := currentProductReader().(store.OrderAdminReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "order administration unavailable"})
		return
	}
	if err := writer.FulfillAdminOrder(r.Context(), session.ShopID, r.PathValue("id"), input.Tracking); err != nil {
		writeOrderAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeOrderAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writePrivateJSON(w, http.StatusNotFound, map[string]any{"error": "order not found"})
	case errors.Is(err, store.ErrOrderTransition):
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "order cannot make this transition in its current state"})
	default:
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func adminStaffList(w http.ResponseWriter, r *http.Request) {
	session, _ := adminSessionFromContext(r.Context())
	manager, ok := currentProductReader().(store.StaffMemberWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "staff management unavailable"})
		return
	}
	members, err := manager.ListStaffMembers(r.Context(), session.ShopID)
	if err != nil {
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": members})
}

func adminStaffCreate(w http.ResponseWriter, r *http.Request) {
	var input store.StaffMemberInput
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	if err := validateStaffMemberInput(input, true); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	manager, ok := currentProductReader().(store.StaffMemberWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "staff management unavailable"})
		return
	}
	member, err := manager.CreateStaffMember(r.Context(), session.ShopID, input)
	if err != nil {
		writeStaffWriteError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusCreated, map[string]any{"data": member})
}

func adminStaffUpdate(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "staff member id is required"})
		return
	}
	var input store.StaffMemberInput
	if err := decodeAdminJSON(w, r, &input); err != nil {
		writeAdminInputError(w, err)
		return
	}
	if err := validateStaffMemberInput(input, false); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	manager, ok := currentProductReader().(store.StaffMemberWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "staff management unavailable"})
		return
	}
	member, err := manager.UpdateStaffMember(r.Context(), session.ShopID, id, input)
	if err != nil {
		writeStaffWriteError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": member})
}

func adminStaffRemove(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "staff member id is required"})
		return
	}
	session, _ := adminSessionFromContext(r.Context())
	manager, ok := currentProductReader().(store.StaffMemberWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "staff management unavailable"})
		return
	}
	if err := manager.RemoveStaffMember(r.Context(), session.ShopID, id); err != nil {
		writeStaffWriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validateStaffMemberInput(input store.StaffMemberInput, requirePassword bool) error {
	email := strings.TrimSpace(input.Email)
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(parsedEmail.Address, email) || len(email) > 254 {
		return errors.New("a valid email address is required")
	}
	if len(strings.TrimSpace(input.Name)) == 0 || len(input.Name) > 120 {
		return errors.New("staff name is required and must be at most 120 bytes")
	}
	if input.Role != "admin" && input.Role != "editor" && input.Role != "support" {
		return errors.New("role must be admin, editor, or support")
	}
	if requirePassword && len(input.Password) == 0 {
		return errors.New("an initial password is required")
	}
	if input.Password != "" && (len(input.Password) < 12 || len(input.Password) > 72) {
		return errors.New("password must contain 12 to 72 bytes")
	}
	return nil
}

func writeStaffWriteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writePrivateJSON(w, http.StatusNotFound, map[string]any{"error": "staff member not found"})
	case errors.Is(err, store.ErrStaffAlreadyExists), errors.Is(err, store.ErrStaffOwnerProtected):
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "staff member conflicts with existing data"})
	case errors.Is(err, store.ErrStaffRoleInvalid):
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid staff role"})
	default:
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

var catalogSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var catalogLocalePattern = regexp.MustCompile(`^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$`)
var catalogSKU = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var catalogCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
var catalogAmount = regexp.MustCompile(`^(?:0|[1-9][0-9]{0,14})(?:\.[0-9]{1,4})?$`)
var catalogMediaTypes = map[string]struct{}{
	"image/avif": {}, "image/gif": {}, "image/jpeg": {}, "image/png": {},
	"image/svg+xml": {}, "image/webp": {},
}

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
		if variant.WeightGrams != nil && (*variant.WeightGrams < 0 || *variant.WeightGrams > 2147483647) {
			return errors.New("variant weight_grams must be between 0 and 2147483647")
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
	if len(input.Media) > 20 {
		return errors.New("at most 20 product media items are allowed")
	}
	mediaURIs := make(map[string]struct{}, len(input.Media))
	for _, media := range input.Media {
		if !validCatalogMediaURI(media.URI) {
			return errors.New("media URI must be an HTTPS URL or a storefront image path")
		}
		if _, exists := catalogMediaTypes[media.MimeType]; !exists {
			return errors.New("media must use a supported image MIME type")
		}
		if len(media.Alt) > 500 {
			return errors.New("media alt text must be at most 500 bytes")
		}
		if _, exists := mediaURIs[media.URI]; exists {
			return errors.New("media URIs must be unique per product")
		}
		mediaURIs[media.URI] = struct{}{}
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

func validCatalogMediaURI(raw string) bool {
	if len(raw) == 0 || len(raw) > 2048 || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "\\\r\n\t ") {
		return false
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.User != nil {
		return false
	}
	if parsed.Scheme == "https" {
		return parsed.Host != "" && parsed.Opaque == ""
	}
	if parsed.Scheme != "" || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/storefront/images/") {
		return false
	}
	return !strings.Contains(parsed.Path, "/../") && !strings.HasSuffix(parsed.Path, "/..")
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
