package market

import (
	"regexp"
	"testing"
)

func TestEUCountriesReference(t *testing.T) {
	countries := EUCountries()
	if len(countries) != 27 {
		t.Fatalf("EU countries count = %d, want 27", len(countries))
	}

	codes := make(map[string]bool, len(countries))
	for _, country := range countries {
		if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(country.Code) {
			t.Errorf("country code %q is not ISO alpha-2 shaped", country.Code)
		}
		if codes[country.Code] {
			t.Errorf("duplicate country code %q", country.Code)
		}
		codes[country.Code] = true
		if country.Name == "" || !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(country.Currency) {
			t.Errorf("country metadata incomplete: %+v", country)
		}
		if len(country.LocaleCandidates) == 0 {
			t.Errorf("country %s has no locale candidates", country.Code)
		}
		for _, locale := range country.LocaleCandidates {
			if !regexp.MustCompile(`^[a-z]{2,3}-[A-Z]{2}$`).MatchString(locale) {
				t.Errorf("country %s has invalid locale candidate %q", country.Code, locale)
			}
		}
	}
	for _, code := range []string{"AT", "BE", "BG", "CY", "CZ", "DE", "DK", "EE", "ES", "FI", "FR", "GR", "HR", "HU", "IE", "IT", "LT", "LU", "LV", "MT", "NL", "PL", "PT", "RO", "SE", "SI", "SK"} {
		if !codes[code] {
			t.Errorf("EU country reference is missing %s", code)
		}
	}
}

func TestEUCountriesReturnsIndependentLocaleSlices(t *testing.T) {
	first := EUCountries()
	first[0].LocaleCandidates[0] = "xx-XX"

	second := EUCountries()
	if second[0].LocaleCandidates[0] == "xx-XX" {
		t.Fatal("mutating a returned country changed the shared reference")
	}
}
