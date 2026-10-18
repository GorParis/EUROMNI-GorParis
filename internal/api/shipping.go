package api

import (
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strings"

	"euromni/internal/seed"
	"euromni/internal/store"
)

var shippingCodePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var shippingAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,14})(\.[0-9]{1,4})?$`)
var shippingQuoteIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
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

func shippingQuote(w http.ResponseWriter, r *http.Request) {
	reader, tokenHash, ok := authenticatedCartReader(r)
	if !ok {
		writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "cart authorization required"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCartRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Country string `json:"country"`
	}
	if err := decoder.Decode(&request); err != nil {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid shipping quote request"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "shipping quote request must contain one JSON object"})
		return
	}
	country := strings.ToUpper(strings.TrimSpace(request.Country))
	if !validCountryCode(country) {
		writePrivateJSON(w, http.StatusBadRequest, map[string]any{"error": "a two-letter country code is required"})
		return
	}
	quoteReader, ok := reader.(store.ShippingQuoteReader)
	if !ok {
		writePrivateJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "shipping quotes unavailable"})
		return
	}
	options, err := quoteReader.ListCartShippingOptions(r.Context(), seed.ShopID, tokenHash, country)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrInvalidCartToken):
			writePrivateJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid cart authorization"})
		case errors.Is(err, store.ErrNotFound):
			writePrivateJSON(w, http.StatusConflict, map[string]any{"error": "cart contains unavailable items"})
		default:
			writePrivateJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
		}
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
	totalRules := 0
	for index := range config.Rates {
		rate := &config.Rates[index]
		rate.CarrierCode = strings.TrimSpace(rate.CarrierCode)
		rate.ZoneCode = strings.TrimSpace(rate.ZoneCode)
		rate.Amount = strings.TrimSpace(rate.Amount)
		rate.Currency = strings.TrimSpace(rate.Currency)
		rate.Mode = strings.TrimSpace(rate.Mode)
		if rate.Mode == "" {
			rate.Mode = "fixed"
		}
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
		if rate.Mode != "fixed" && rate.Mode != "weight_value" {
			return "rate mode must be fixed or weight_value"
		}
		if !shippingCurrencyPattern.MatchString(rate.Currency) {
			return "currencies must use three uppercase letters"
		}
		switch rate.Mode {
		case "fixed":
			if !shippingAmountPattern.MatchString(rate.Amount) {
				return "shipping amounts must be non-negative numbers with at most four decimal places"
			}
			if len(rate.Rules) > 0 {
				return "fixed rates cannot contain weight and subtotal rules"
			}
			rate.Rules = []store.ShippingRateRule{}
		case "weight_value":
			if len(rate.Rules) == 0 || len(rate.Rules) > 200 {
				return "weight_value rates require 1 to 200 rules"
			}
			totalRules += len(rate.Rules)
			if totalRules > 5000 {
				return "shipping configuration exceeds the supported rule limit"
			}
			for ruleIndex := range rate.Rules {
				rule := &rate.Rules[ruleIndex]
				rule.MinSubtotal = strings.TrimSpace(rule.MinSubtotal)
				rule.Amount = strings.TrimSpace(rule.Amount)
				if rule.MinSubtotal == "" {
					rule.MinSubtotal = "0"
				}
				if rule.MinWeightGrams < 0 ||
					(rule.MaxWeightGrams != nil && *rule.MaxWeightGrams <= rule.MinWeightGrams) {
					return "weight ranges must be non-negative and have an exclusive upper bound greater than the lower bound"
				}
				if !shippingAmountPattern.MatchString(rule.MinSubtotal) ||
					(rule.MaxSubtotal != nil && !shippingAmountPattern.MatchString(strings.TrimSpace(*rule.MaxSubtotal))) ||
					!shippingAmountPattern.MatchString(rule.Amount) {
					return "weight_value rules require valid subtotal bounds and a non-negative amount with at most four decimal places"
				}
				if rule.MaxSubtotal != nil {
					upper := strings.TrimSpace(*rule.MaxSubtotal)
					rule.MaxSubtotal = &upper
					minimum, _ := new(big.Rat).SetString(rule.MinSubtotal)
					maximum, _ := new(big.Rat).SetString(upper)
					if maximum.Cmp(minimum) <= 0 {
						return "subtotal ranges must have an exclusive upper bound greater than the lower bound"
					}
				}
				for previous := 0; previous < ruleIndex; previous++ {
					if shippingRulesOverlap(rate.Rules[previous], *rule) {
						return "weight_value rules cannot overlap"
					}
				}
			}
		}
	}
	return ""
}

func shippingRulesOverlap(left, right store.ShippingRateRule) bool {
	weightOverlap := (left.MaxWeightGrams == nil || right.MinWeightGrams < *left.MaxWeightGrams) &&
		(right.MaxWeightGrams == nil || left.MinWeightGrams < *right.MaxWeightGrams)
	if !weightOverlap {
		return false
	}
	leftMin, _ := new(big.Rat).SetString(left.MinSubtotal)
	rightMin, _ := new(big.Rat).SetString(right.MinSubtotal)
	subtotalOverlap := left.MaxSubtotal == nil || rightMin.Cmp(mustShippingRat(*left.MaxSubtotal)) < 0
	subtotalOverlap = subtotalOverlap && (right.MaxSubtotal == nil || leftMin.Cmp(mustShippingRat(*right.MaxSubtotal)) < 0)
	return subtotalOverlap
}

func mustShippingRat(value string) *big.Rat {
	rational, _ := new(big.Rat).SetString(value)
	return rational
}
