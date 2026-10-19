package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"

	"euromni/internal/seed"
	"euromni/internal/store"
)

type checkoutRequest struct {
	Email                   string                `json:"email"`
	Name                    string                `json:"name"`
	ShippingCarrierCode     string                `json:"shipping_carrier_code"`
	ShippingServiceCode     string                `json:"shipping_service_code,omitempty"`
	ExpectedShippingTotal   string                `json:"expected_shipping_total,omitempty"`
	ExpectedShippingQuoteID string                `json:"expected_shipping_quote_id,omitempty"`
	ShippingAddress         store.ShippingAddress `json:"shipping_address"`
}

func checkout(w http.ResponseWriter, r *http.Request) {
	reader, tokenHash, ok := authenticatedCartReader(r)
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "cart authorization required"})
		return
	}
	details, err := decodeCheckoutRequest(w, r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writePrivateJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "request body too large"})
		} else {
			writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid checkout details"})
		}
		return
	}
	if _, ok := reader.(store.ShippingOptionReader); !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "shipping service unavailable"})
		return
	}
	writer, ok := reader.(store.CheckoutWriter)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "checkout service unavailable"})
		return
	}
	order, err := writer.CheckoutCart(r.Context(), seed.ShopID, tokenHash, details)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidCartToken):
			writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid cart authorization"})
		case errors.Is(err, store.ErrOutOfStock):
			writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "insufficient inventory"})
		case errors.Is(err, store.ErrEmptyCart):
			writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "cart is empty"})
		case errors.Is(err, store.ErrShippingUnavailable):
			writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "shipping method unavailable for this destination"})
		case errors.Is(err, store.ErrShippingQuoteChanged):
			writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "shipping quote changed"})
		case errors.Is(err, store.ErrInvalidPersonalization):
			writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "product personalization changed; review your cart"})
		case errors.Is(err, store.ErrNotFound):
			writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "cart contains unavailable items"})
		default:
			writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		}
		return
	}
	writePrivateJSON(w, http.StatusCreated, map[string]any{"data": order})
}

func decodeCheckoutRequest(w http.ResponseWriter, r *http.Request) (store.CheckoutDetails, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCartRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request checkoutRequest
	if err := decoder.Decode(&request); err != nil {
		return store.CheckoutDetails{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return store.CheckoutDetails{}, errors.New("multiple JSON values")
		}
		return store.CheckoutDetails{}, err
	}
	address := request.ShippingAddress
	address.Line1 = strings.TrimSpace(address.Line1)
	address.Line2 = strings.TrimSpace(address.Line2)
	address.City = strings.TrimSpace(address.City)
	address.PostalCode = strings.TrimSpace(address.PostalCode)
	address.Region = strings.TrimSpace(address.Region)
	address.Country = strings.ToUpper(strings.TrimSpace(address.Country))
	name := strings.TrimSpace(request.Name)
	carrierCode := strings.TrimSpace(request.ShippingCarrierCode)
	serviceCode := strings.TrimSpace(request.ShippingServiceCode)
	if serviceCode == "" {
		serviceCode = "legacy"
	}
	expectedShippingTotal := strings.TrimSpace(request.ExpectedShippingTotal)
	expectedShippingQuoteID := strings.TrimSpace(request.ExpectedShippingQuoteID)
	parsedEmail, err := mail.ParseAddress(strings.TrimSpace(request.Email))
	if err != nil || parsedEmail.Address == "" || len(parsedEmail.Address) > 320 || len(name) == 0 || len(name) > 120 ||
		!shippingCodePattern.MatchString(carrierCode) ||
		!shippingCodePattern.MatchString(serviceCode) ||
		len(address.Line1) == 0 || len(address.Line1) > 200 || len(address.Line2) > 200 ||
		len(address.City) == 0 || len(address.City) > 100 || len(address.PostalCode) == 0 || len(address.PostalCode) > 32 ||
		len(address.Region) > 100 || !validCountryCode(address.Country) {
		return store.CheckoutDetails{}, errors.New("invalid checkout details")
	}
	if expectedShippingTotal != "" && !shippingAmountPattern.MatchString(expectedShippingTotal) {
		return store.CheckoutDetails{}, errors.New("invalid expected shipping total")
	}
	if expectedShippingQuoteID != "" && !shippingQuoteIDPattern.MatchString(expectedShippingQuoteID) {
		return store.CheckoutDetails{}, errors.New("invalid expected shipping quote id")
	}
	return store.CheckoutDetails{
		Email: parsedEmail.Address, Name: name, ShippingCarrierCode: carrierCode,
		ShippingServiceCode:   serviceCode,
		ExpectedShippingTotal: expectedShippingTotal, ExpectedShippingQuoteID: expectedShippingQuoteID,
		ShippingAddress: address,
	}, nil
}

func validCountryCode(country string) bool {
	if len(country) != 2 {
		return false
	}
	return country[0] >= 'A' && country[0] <= 'Z' && country[1] >= 'A' && country[1] <= 'Z'
}
