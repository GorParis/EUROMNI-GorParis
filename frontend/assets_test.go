package frontend

import (
	"io/fs"
	"strings"
	"testing"
)

func TestEmbeddedStorefrontAndAdminAssets(t *testing.T) {
	tests := []struct {
		name string
		fsys fs.FS
		path string
	}{
		{name: "storefront page", fsys: Assets, path: "index.html"},
		{name: "storefront component", fsys: Assets, path: "euromni-catalog.js"},
		{name: "cabas image", fsys: Assets, path: "images/cabas-lin.svg"},
		{name: "belt image", fsys: Assets, path: "images/ceinture-cuir.svg"},
		{name: "shirt image", fsys: Assets, path: "images/chemise-lin.svg"},
		{name: "cup image", fsys: Assets, path: "images/tasse-gres.svg"},
		{name: "candle image", fsys: Assets, path: "images/bougie-cedre.svg"},
		{name: "admin page", fsys: AdminAssets, path: "admin/index.html"},
		{name: "admin script", fsys: AdminAssets, path: "admin/admin.js"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info, err := fs.Stat(test.fsys, test.path)
			if err != nil {
				t.Fatalf("embedded asset %q is unavailable: %v", test.path, err)
			}
			if info.IsDir() || info.Size() == 0 {
				t.Errorf("embedded asset %q must be a non-empty file", test.path)
			}
		})
	}
}

func TestEmbeddedFrontendContainsShippingCheckoutFlow(t *testing.T) {
	catalog, err := fs.ReadFile(Assets, "euromni-catalog.js")
	if err != nil {
		t.Fatalf("read storefront component: %v", err)
	}
	for _, feature := range []string{
		"/v1/shipping-options",
		"/v1/reference/eu-countries",
		"Choisissez un pays de livraison",
		"aucun change automatique",
		"Langue du contenu",
		"euromni.locale",
		"refreshAvailableLocales",
		"translationForEntries",
		"shipping_carrier_code",
		"checkout-estimate",
		"shipping method unavailable for this destination",
	} {
		if !strings.Contains(string(catalog), feature) {
			t.Errorf("storefront component is missing shipping checkout feature %q", feature)
		}
	}
	admin, err := fs.ReadFile(AdminAssets, "admin/admin.js")
	if err != nil {
		t.Fatalf("read admin script: %v", err)
	}
	for _, feature := range []string{
		"/v1/admin/shipping",
		"Configuration transport enregistrée.",
		"country_codes",
		"carrier_code",
		"/payments/manual",
		"provider_reference",
		"/fulfill",
		"Confirmer le paiement reçu",
		"Marquer comme expédiée",
	} {
		if !strings.Contains(string(admin), feature) {
			t.Errorf("admin script is missing shipping configuration feature %q", feature)
		}
	}
}
