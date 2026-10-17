package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"euromni/internal/seed"
	"euromni/internal/store"
)

var shippingCodePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var shippingAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,14})(\.[0-9]{1,4})?$`)
var shippingCountryPattern = regexp.MustCompile(`^[A-Z]{2}$`)
var shippingCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func shippingOptions(w http.ResponseWriter, r *http.Request) {
	country := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("country")))
	if !validCountryCode(country) {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "a two-letter country code is required"})
		return
	}
	reader, ok := currentProductReader().(store.ShippingOptionReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "shipping options unavailable"})
		return
	}
	options, err := reader.ListShippingOptions(r.Context(), seed.ShopID, country)
	if err != nil {
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": options})
}

func adminShippingConfig(w http.ResponseWriter, r *http.Request) {
	session, _ := adminSessionFromContext(r.Context())
	reader, ok := currentProductReader().(store.ShippingConfigReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "shipping configuration unavailable"})
		return
	}
	if r.Method == http.MethodGet {
		config, err := reader.GetShippingConfig(r.Context(), session.ShopID)
		if err != nil {
			writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			return
		}
		writePrivateJSON(w, http.StatusOK, map[string]any{"data": config})
		return
	}
	if r.Method != http.MethodPut {
		writePrivateJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var config *store.ShippingConfig
	if err := decoder.Decode(&config); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid shipping configuration"})
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "shipping configuration must contain one JSON object"})
		return
	}
	if config == nil || config.Carriers == nil || config.Zones == nil || config.Rates == nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "shipping configuration must include carriers, zones and rates arrays"})
		return
	}
	if message := validateShippingConfig(config); message != "" {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": message})
		return
	}
	if err := reader.ReplaceShippingConfig(r.Context(), session.ShopID, *config); err != nil {
		writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "shipping configuration could not be saved"})
		return
	}
	writePrivateJSON(w, http.StatusOK, map[string]any{"data": config})
}

func validateShippingConfig(config *store.ShippingConfig) string {
	if len(config.Carriers) > 100 || len(config.Zones) > 100 || len(config.Rates) > 1000 {
		return "shipping configuration exceeds the supported limits"
	}
	carriers := make(map[string]struct{}, len(config.Carriers))
	for index := range config.Carriers {
		carrier := &config.Carriers[index]
		carrier.Code = strings.TrimSpace(carrier.Code)
		carrier.Name = strings.TrimSpace(carrier.Name)
		if !shippingCodePattern.MatchString(carrier.Code) {
			return "carrier codes must use lowercase letters, numbers and single hyphens"
		}
		if carrier.Name == "" || len(carrier.Name) > 120 {
			return "carrier names must contain 1 to 120 characters"
		}
		if _, exists := carriers[carrier.Code]; exists {
			return "carrier codes must be unique"
		}
		carriers[carrier.Code] = struct{}{}
	}
	zones := make(map[string]struct{}, len(config.Zones))
	countries := make(map[string]string)
	for index := range config.Zones {
		zone := &config.Zones[index]
		zone.Code = strings.TrimSpace(zone.Code)
		zone.Name = strings.TrimSpace(zone.Name)
		if !shippingCodePattern.MatchString(zone.Code) {
			return "zone codes must use lowercase letters, numbers and single hyphens"
		}
		if zone.Name == "" || len(zone.Name) > 120 {
			return "zone names must contain 1 to 120 characters"
		}
		if _, exists := zones[zone.Code]; exists {
			return "zone codes must be unique"
		}
		if len(zone.CountryCodes) == 0 || len(zone.CountryCodes) > 250 {
			return "each zone must contain 1 to 250 country codes"
		}
		zones[zone.Code] = struct{}{}
		seenCountries := make(map[string]struct{}, len(zone.CountryCodes))
		for countryIndex, country := range zone.CountryCodes {
			country = strings.TrimSpace(country)
			if !shippingCountryPattern.MatchString(country) {
				return "country codes must use two uppercase letters"
			}
			if _, exists := seenCountries[country]; exists {
				return "country codes must be unique within a zone"
			}
			if _, exists := countries[country]; exists {
				return "a country cannot belong to more than one shipping zone"
			}
			seenCountries[country] = struct{}{}
			countries[country] = zone.Code
			zone.CountryCodes[countryIndex] = country
		}
	}
	rates := make(map[string]struct{}, len(config.Rates))
	for index := range config.Rates {
		rate := &config.Rates[index]
		rate.CarrierCode = strings.TrimSpace(rate.CarrierCode)
		rate.ZoneCode = strings.TrimSpace(rate.ZoneCode)
		rate.Amount = strings.TrimSpace(rate.Amount)
		rate.Currency = strings.TrimSpace(rate.Currency)
		if _, exists := carriers[rate.CarrierCode]; !exists {
			return "each rate must refer to a configured carrier"
		}
		if _, exists := zones[rate.ZoneCode]; !exists {
			return "each rate must refer to a configured zone"
		}
		key := rate.CarrierCode + "\x00" + rate.ZoneCode
		if _, exists := rates[key]; exists {
			return "each carrier can have only one rate per zone"
		}
		rates[key] = struct{}{}
		if !shippingAmountPattern.MatchString(rate.Amount) {
			return "shipping amounts must be non-negative numbers with at most four decimal places"
		}
		if !shippingCurrencyPattern.MatchString(rate.Currency) {
			return "currencies must use three uppercase letters"
		}
	}
	return ""
}
