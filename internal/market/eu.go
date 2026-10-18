package market

type EUCountry struct {
	Code             string   `json:"code"`
	Name             string   `json:"name"`
	Currency         string   `json:"currency"`
	LocaleCandidates []string `json:"locale_candidates"`
}

var euCountries = []EUCountry{
	{Code: "DE", Name: "Allemagne", Currency: "EUR", LocaleCandidates: []string{"de-DE"}},
	{Code: "AT", Name: "Autriche", Currency: "EUR", LocaleCandidates: []string{"de-AT"}},
	{Code: "BE", Name: "Belgique", Currency: "EUR", LocaleCandidates: []string{"nl-BE", "fr-BE", "de-BE"}},
	{Code: "BG", Name: "Bulgarie", Currency: "EUR", LocaleCandidates: []string{"bg-BG"}},
	{Code: "CY", Name: "Chypre", Currency: "EUR", LocaleCandidates: []string{"el-CY", "tr-CY"}},
	{Code: "HR", Name: "Croatie", Currency: "EUR", LocaleCandidates: []string{"hr-HR"}},
	{Code: "DK", Name: "Danemark", Currency: "DKK", LocaleCandidates: []string{"da-DK"}},
	{Code: "ES", Name: "Espagne", Currency: "EUR", LocaleCandidates: []string{"es-ES", "ca-ES", "eu-ES", "gl-ES"}},
	{Code: "EE", Name: "Estonie", Currency: "EUR", LocaleCandidates: []string{"et-EE"}},
	{Code: "FI", Name: "Finlande", Currency: "EUR", LocaleCandidates: []string{"fi-FI", "sv-FI"}},
	{Code: "FR", Name: "France", Currency: "EUR", LocaleCandidates: []string{"fr-FR"}},
	{Code: "GR", Name: "Grèce", Currency: "EUR", LocaleCandidates: []string{"el-GR"}},
	{Code: "HU", Name: "Hongrie", Currency: "HUF", LocaleCandidates: []string{"hu-HU"}},
	{Code: "IE", Name: "Irlande", Currency: "EUR", LocaleCandidates: []string{"en-IE", "ga-IE"}},
	{Code: "IT", Name: "Italie", Currency: "EUR", LocaleCandidates: []string{"it-IT"}},
	{Code: "LV", Name: "Lettonie", Currency: "EUR", LocaleCandidates: []string{"lv-LV"}},
	{Code: "LT", Name: "Lituanie", Currency: "EUR", LocaleCandidates: []string{"lt-LT"}},
	{Code: "LU", Name: "Luxembourg", Currency: "EUR", LocaleCandidates: []string{"lb-LU", "fr-LU", "de-LU"}},
	{Code: "MT", Name: "Malte", Currency: "EUR", LocaleCandidates: []string{"mt-MT", "en-MT"}},
	{Code: "NL", Name: "Pays-Bas", Currency: "EUR", LocaleCandidates: []string{"nl-NL"}},
	{Code: "PL", Name: "Pologne", Currency: "PLN", LocaleCandidates: []string{"pl-PL"}},
	{Code: "PT", Name: "Portugal", Currency: "EUR", LocaleCandidates: []string{"pt-PT"}},
	{Code: "RO", Name: "Roumanie", Currency: "RON", LocaleCandidates: []string{"ro-RO"}},
	{Code: "SK", Name: "Slovaquie", Currency: "EUR", LocaleCandidates: []string{"sk-SK"}},
	{Code: "SI", Name: "Slovénie", Currency: "EUR", LocaleCandidates: []string{"sl-SI"}},
	{Code: "SE", Name: "Suède", Currency: "SEK", LocaleCandidates: []string{"sv-SE"}},
	{Code: "CZ", Name: "Tchéquie", Currency: "CZK", LocaleCandidates: []string{"cs-CZ"}},
}

func EUCountries() []EUCountry {
	countries := make([]EUCountry, len(euCountries))
	for index, country := range euCountries {
		countries[index] = country
		countries[index].LocaleCandidates = append([]string(nil), country.LocaleCandidates...)
	}
	return countries
}
