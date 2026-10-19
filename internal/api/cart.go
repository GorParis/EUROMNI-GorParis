package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"euromni/internal/seed"
	"euromni/internal/store"
)

const maxCartRequestBytes = 16 << 10

type cartAddRequest struct {
	VariantID        string            `json:"variant_id"`
	Quantity         int               `json:"quantity"`
	Personalizations map[string]string `json:"personalizations,omitempty"`
}

func cartCreate(w http.ResponseWriter, r *http.Request) {
	reader, ok := currentProductReader().(store.CartReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "cart service unavailable"})
		return
	}
	token, err := createCartToken(r.Context(), reader)
	if err != nil {
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusCreated, map[string]any{"data": map[string]string{"token": token}})
}

func createCartToken(ctx context.Context, reader store.CartReader) (string, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	tokenHash := sha256.Sum256(rawToken)
	if err := reader.CreateCart(ctx, seed.ShopID, tokenHash[:]); err != nil {
		return "", err
	}
	return token, nil
}

func cartView(w http.ResponseWriter, r *http.Request) {
	reader, tokenHash, ok := authenticatedCartReader(r)
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "cart authorization required"})
		return
	}
	cart, err := reader.GetCart(r.Context(), seed.ShopID, tokenHash)
	if err != nil {
		writeCartError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": cart})
}

func cartAdd(w http.ResponseWriter, r *http.Request) {
	reader, tokenHash, ok := authenticatedCartReader(r)
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "cart authorization required"})
		return
	}
	request, err := decodeCartAddRequest(w, r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body too large"})
		} else {
			writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid cart item"})
		}
		return
	}
	if request.VariantID == "" || request.Quantity < 1 || request.Quantity > 99 {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid cart item"})
		return
	}
	cart, err := reader.AddCartLine(r.Context(), seed.ShopID, tokenHash, request.VariantID, request.Quantity, request.Personalizations)
	if err != nil {
		writeCartError(w, err)
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": cart})
}

func authenticatedCartReader(r *http.Request) (store.CartReader, []byte, bool) {
	return cartReaderFromAuthorization(r.Header.Get("Authorization"))
}

func cartReaderFromAuthorization(authorization string) (store.CartReader, []byte, bool) {
	reader, ok := currentProductReader().(store.CartReader)
	if !ok {
		return nil, nil, false
	}
	scheme, token, ok := strings.Cut(authorization, " ")
	if !ok || scheme != "Bearer" || len(token) != base64.RawURLEncoding.EncodedLen(sha256.Size) || strings.ContainsAny(token, " \t\r\n") {
		return nil, nil, false
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(rawToken) != sha256.Size {
		return nil, nil, false
	}
	tokenHash := sha256.Sum256(rawToken)
	return reader, tokenHash[:], true
}

func decodeCartAddRequest(w http.ResponseWriter, r *http.Request) (cartAddRequest, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCartRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request cartAddRequest
	if err := decoder.Decode(&request); err != nil {
		return cartAddRequest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return cartAddRequest{}, errors.New("multiple JSON values")
		}
		return cartAddRequest{}, err
	}
	return request, nil
}

func writeCartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalidCartToken):
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid cart authorization"})
	case errors.Is(err, store.ErrInvalidCartQuantity):
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid cart quantity"})
	case errors.Is(err, store.ErrInvalidPersonalization):
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid product personalization"})
	case errors.Is(err, store.ErrOutOfStock):
		writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "insufficient inventory"})
	case errors.Is(err, store.ErrNotFound):
		writePrivateJSON(w, http.StatusNotFound, map[string]any{"error": "cart item not found"})
	default:
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
	}
}

func writePrivateJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, status, payload)
}
