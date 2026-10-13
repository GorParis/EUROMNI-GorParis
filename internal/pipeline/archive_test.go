package pipeline

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePHPArchiveAcceptsWellFormedPackage(t *testing.T) {
	payload := buildZIP(t, map[string]string{
		"manifest.json":  `{"name":"demo-woo-module","version":"1.0.0","platform":"woocommerce","shop":"atelier-demo"}`,
		"src/plugin.php": `<?php\nadd_action('init', function () { return true; });\n`,
		"README.md":      "# Demo PHP module\n",
	})

	if err := ValidatePHPArchive(payload); err != nil {
		t.Fatalf("ValidatePHPArchive() returned error for a valid archive: %v", err)
	}
}

func TestValidatePHPArchiveRejectsTraversalAndUnexpectedFiles(t *testing.T) {
	payload := buildZIP(t, map[string]string{
		"manifest.json":   `{"name":"evil","version":"1.0.0","platform":"woocommerce"}`,
		"../src/evil.php": `<?php echo 'pwned';`,
		"evil.sh":         "#!/bin/sh\n",
	})

	if err := ValidatePHPArchive(payload); err == nil {
		t.Fatal("ValidatePHPArchive() accepted an unsafe archive")
	}
}

func TestParsePHPManifestAcceptsSupportedModuleContract(t *testing.T) {
	payload := buildZIP(t, map[string]string{
		"manifest.json": `{"name":"demo-woo-module","version":"1.0.0","platform":"woocommerce","shop":"atelier-demo","source":{"type":"zip"}}`,
		"src/plugin.php": `<?php
add_action('init', function () { return true; });
`,
	})

	manifest, err := ParsePHPManifest(payload)
	if err != nil {
		t.Fatalf("ParsePHPManifest() returned error: %v", err)
	}
	if manifest.Name != "demo-woo-module" || manifest.Version != "1.0.0" || manifest.Platform != "woocommerce" {
		t.Fatalf("manifest = %+v, want expected demo module metadata", manifest)
	}
	if manifest.Shop != "atelier-demo" {
		t.Fatalf("manifest shop = %q, want atelier-demo", manifest.Shop)
	}
}

func TestParsePHPManifestRejectsMissingOrInvalidMetadata(t *testing.T) {
	payload := buildZIP(t, map[string]string{
		"manifest.json":  `{"name":"broken","platform":"woocommerce"}`,
		"src/plugin.php": `<?php return true;`,
	})

	if _, err := ParsePHPManifest(payload); err == nil {
		t.Fatal("ParsePHPManifest() accepted a manifest missing required fields")
	}
}

func TestAnalyzePHPSourceFindsWooHooksAndCapabilities(t *testing.T) {
	source := `<?php
/**
 * Plugin Name: Demo Woo Module
 * Version: 1.2.3
 */

add_action('woocommerce_before_calculate_totals', function ($cart) {
    $cart->set_total(42);
});

add_filter('woocommerce_product_data_tabs', function ($tabs) {
    $tabs['demo'] = 'Demo';
    return $tabs;
});

if (isset($_POST['admin'])) {
    wp_die('blocked');
}
`

	analysis, err := AnalyzePHPSource(source)
	if err != nil {
		t.Fatalf("AnalyzePHPSource() returned error: %v", err)
	}
	if analysis.Platform != "woocommerce" {
		t.Fatalf("analysis platform = %q, want woocommerce", analysis.Platform)
	}
	if !contains(analysis.Hooks, "woocommerce_before_calculate_totals") || !contains(analysis.Hooks, "woocommerce_product_data_tabs") {
		t.Fatalf("analysis hooks = %#v, want WooCommerce hook names", analysis.Hooks)
	}
	if !contains(analysis.Capabilities, "cart") {
		t.Fatalf("analysis capabilities = %#v, want cart capability", analysis.Capabilities)
	}
}

func TestAnalyzePHPSourceRecognizesGatewayAndCheckoutHooks(t *testing.T) {
	source := `<?php

add_filter('woocommerce_gateway_description', function ($description) {
    return $description;
});

add_filter('woocommerce_stripe_payment_method_icons', function ($icons) {
    return $icons;
});
`

	analysis, err := AnalyzePHPSource(source)
	if err != nil {
		t.Fatalf("AnalyzePHPSource() returned error for real WooCommerce hooks: %v", err)
	}
	if !contains(analysis.Capabilities, "order") {
		t.Fatalf("analysis capabilities = %#v, want order capability for payment/gateway hooks", analysis.Capabilities)
	}
}

func TestAnalyzePHPSourceFindsPrestaShopRegisteredHooks(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		wantHooks []string
		wantCaps  []string
	}{
		{
			name: "literal registrations",
			source: `<?php
class Ps_EmailAlerts extends Module {
    public function install() {
        return $this->registerHook('actionValidateOrder')
            && $this->registerHook('actionProductDelete')
            && $this->registerHook('displayCustomerAccount');
    }
}`,
			wantHooks: []string{"actionValidateOrder", "actionProductDelete", "displayCustomerAccount"},
			wantCaps:  []string{"order", "catalog", "customer"},
		},
		{
			name: "constant hook list",
			source: `<?php
class Ps_Cashondelivery extends PaymentModule {
    const HOOKS = ['displayOrderConfirmation', 'paymentOptions'];

    public function install() {
        return parent::install() && (bool) $this->registerHook(static::HOOKS);
    }
}`,
			wantHooks: []string{"displayOrderConfirmation", "paymentOptions"},
			wantCaps:  []string{"order"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analysis, err := AnalyzePHPSource(test.source)
			if err != nil {
				t.Fatalf("AnalyzePHPSource() returned error: %v", err)
			}
			if analysis.Platform != "prestashop" {
				t.Fatalf("analysis platform = %q, want prestashop", analysis.Platform)
			}
			for _, hook := range test.wantHooks {
				if !contains(analysis.Hooks, hook) {
					t.Errorf("analysis hooks = %#v, want %q", analysis.Hooks, hook)
				}
			}
			for _, capability := range test.wantCaps {
				if !contains(analysis.Capabilities, capability) {
					t.Errorf("analysis capabilities = %#v, want %q", analysis.Capabilities, capability)
				}
			}
		})
	}
}

func TestAnalyzeRealPrestaShopFixtures(t *testing.T) {
	modules := []struct {
		name            string
		fixtureGlob     string
		wantHooks       []string
		wantHookDetails []PHPHook
		wantCapability  string
	}{
		{
			name:        "ps_emailalerts",
			fixtureGlob: "../../pipeline/fixtures/prestashop-ps_emailalerts/ps_emailalerts-*/ps_emailalerts.php",
			wantHooks:   []string{"actionValidateOrder", "actionProductDelete", "displayCustomerAccount"},
			wantHookDetails: []PHPHook{
				{Name: "actionValidateOrder", Kind: "action", Handler: "hookActionValidateOrder"},
				{Name: "displayCustomerAccount", Kind: "display", Handler: "hookDisplayCustomerAccount"},
			},
			wantCapability: "order",
		},
		{
			name:        "ps_cashondelivery",
			fixtureGlob: "../../pipeline/fixtures/prestashop-ps_cashondelivery/ps_cashondelivery-*/ps_cashondelivery.php",
			wantHooks:   []string{"displayOrderConfirmation", "paymentOptions"},
			wantHookDetails: []PHPHook{
				{Name: "displayOrderConfirmation", Kind: "display", Handler: "hookDisplayOrderConfirmation"},
				{Name: "paymentOptions", Kind: "other", Handler: "hookPaymentOptions"},
			},
			wantCapability: "order",
		},
	}

	for _, module := range modules {
		t.Run(module.name, func(t *testing.T) {
			paths, err := filepath.Glob(module.fixtureGlob)
			if err != nil {
				t.Fatalf("find fixture: %v", err)
			}
			if len(paths) != 1 {
				t.Fatalf("found %d fixture entrypoints for %q, want exactly one; run pipeline/fetch_real_module_fixtures.sh", len(paths), module.name)
			}
			source, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatalf("read fixture %q: %v", paths[0], err)
			}
			analysis, err := AnalyzePHPSource(string(source))
			if err != nil {
				t.Fatalf("AnalyzePHPSource() returned error: %v", err)
			}
			if analysis.Platform != "prestashop" {
				t.Errorf("analysis platform = %q, want prestashop", analysis.Platform)
			}
			for _, hook := range module.wantHooks {
				if !contains(analysis.Hooks, hook) {
					t.Errorf("analysis hooks = %#v, want %q", analysis.Hooks, hook)
				}
			}
			for _, want := range module.wantHookDetails {
				if !containsHookDetail(analysis.HookDetails, want) {
					t.Errorf("analysis hook details = %#v, want %+v", analysis.HookDetails, want)
				}
			}
			if !contains(analysis.Capabilities, module.wantCapability) {
				t.Errorf("analysis capabilities = %#v, want %q", analysis.Capabilities, module.wantCapability)
			}
		})
	}
}

func containsHookDetail(details []PHPHook, want PHPHook) bool {
	for _, detail := range details {
		if detail == want {
			return true
		}
	}
	return false
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func buildZIP(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}
