package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeShopifyManifest(t *testing.T) {
	tests := []struct {
		name       string
		manifest   string
		wantType   string
		wantTarget string
		wantPoint  string
	}{
		{
			name: "discount function",
			manifest: `api_version = "2025-10"
[[extensions]]
name = "discount-function"
handle = "discount-function-js"
type = "function"

[[extensions.targeting]]
target = "cart.lines.discounts.generate.run"
`,
			wantType:   "function",
			wantTarget: "cart.lines.discounts.generate.run",
		},
		{
			name: "checkout UI extension",
			manifest: `type = "checkout_ui_extension"
name = "po-box-validator"
extension_points = ["Checkout::Dynamic::Render"]
`,
			wantType:  "checkout_ui_extension",
			wantPoint: "Checkout::Dynamic::Render",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analysis, err := AnalyzeShopifyManifest([]byte(test.manifest))
			if err != nil {
				t.Fatalf("AnalyzeShopifyManifest() returned error: %v", err)
			}
			if len(analysis.Extensions) != 1 || analysis.Extensions[0].Type != test.wantType {
				t.Fatalf("extensions = %#v, want one %q extension", analysis.Extensions, test.wantType)
			}
			if test.wantTarget != "" && !contains(analysis.Extensions[0].Targets, test.wantTarget) {
				t.Errorf("targets = %#v, want %q", analysis.Extensions[0].Targets, test.wantTarget)
			}
			if test.wantPoint != "" && !contains(analysis.Extensions[0].ExtensionPoints, test.wantPoint) {
				t.Errorf("extension points = %#v, want %q", analysis.Extensions[0].ExtensionPoints, test.wantPoint)
			}
		})
	}
}

func TestAnalyzeMagentoModule(t *testing.T) {
	moduleXML := []byte(`<config><module name="PayPal_Subscription"/></config>`)
	eventsXML := []byte(`<config><event name="sales_order_save_after"><observer name="save" instance="Vendor\Observer\Save"/></event></config>`)
	diXML := []byte(`<config><type name="Magento\Sales\Model\Order"><plugin name="order-plugin" type="Vendor\Plugin\Order"/></type><preference for="Vendor\Api\OrderInterface" type="Vendor\Model\Order"/></config>`)

	analysis, err := AnalyzeMagentoModule(moduleXML, eventsXML, diXML)
	if err != nil {
		t.Fatalf("AnalyzeMagentoModule() returned error: %v", err)
	}
	if analysis.ModuleName != "PayPal_Subscription" {
		t.Fatalf("module name = %q, want PayPal_Subscription", analysis.ModuleName)
	}
	if !contains(analysis.Events, "sales_order_save_after") {
		t.Fatalf("events = %#v, want sales_order_save_after", analysis.Events)
	}
	if len(analysis.Plugins) != 1 || analysis.Plugins[0].Name != "order-plugin" {
		t.Fatalf("plugins = %#v, want order-plugin", analysis.Plugins)
	}
	if len(analysis.Preferences) != 1 || analysis.Preferences[0].For != `Vendor\Api\OrderInterface` {
		t.Fatalf("preferences = %#v, want the order interface mapping", analysis.Preferences)
	}
	if len(analysis.Arguments) != 0 {
		t.Fatalf("arguments = %#v, want no DI arguments", analysis.Arguments)
	}
}

func TestAnalyzeShopifyManifestRejectsIncompleteExtension(t *testing.T) {
	manifest := []byte(`type = "checkout_ui_extension"
name = "missing-target"
`)
	if _, err := AnalyzeShopifyManifest(manifest); err == nil {
		t.Fatal("AnalyzeShopifyManifest() accepted an extension without a target")
	}
}

func TestAnalyzeMagentoModuleRejectsInvalidModuleMetadata(t *testing.T) {
	tests := []struct {
		name      string
		moduleXML string
	}{
		{name: "missing name", moduleXML: `<config><module/></config>`},
		{name: "malformed XML", moduleXML: `<config><module name="Broken"></config>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AnalyzeMagentoModule([]byte(test.moduleXML), nil, nil); err == nil {
				t.Fatal("AnalyzeMagentoModule() accepted invalid module metadata")
			}
		})
	}
}

func TestAnalyzeRealPlatformFixtures(t *testing.T) {
	t.Run("shopify discount function", func(t *testing.T) {
		manifest := readProjectFixture(t, "../../pipeline/fixtures/real-modules/shopify-function-examples/function-examples-19ccafceda1d0052c2c90c0a1e4db3fe37b16c27/sample-apps/discounts/extensions/product-discount-js/shopify.extension.toml")
		analysis, err := AnalyzeShopifyManifest(manifest)
		if err != nil {
			t.Fatalf("AnalyzeShopifyManifest() returned error: %v", err)
		}
		if len(analysis.Extensions) == 0 || analysis.Extensions[0].Type != "function" || !contains(analysis.Extensions[0].Targets, "purchase.product-discount.run") {
			t.Fatalf("analysis = %#v, want discount function target", analysis)
		}
	})

	t.Run("shopify checkout UI", func(t *testing.T) {
		manifest := readProjectFixture(t, "../../pipeline/fixtures/real-modules/shopify-checkout-ui-extensions-po-box-validator/checkout-ui-extensions-po-box-validator-f1d35d27575b8024113096c3c6f7892ab63a482e/extensions/po-box-validator/shopify.ui.extension.toml")
		analysis, err := AnalyzeShopifyManifest(manifest)
		if err != nil {
			t.Fatalf("AnalyzeShopifyManifest() returned error: %v", err)
		}
		if len(analysis.Extensions) == 0 || analysis.Extensions[0].Type != "checkout_ui_extension" || !contains(analysis.Extensions[0].ExtensionPoints, "Checkout::Dynamic::Render") {
			t.Fatalf("analysis = %#v, want checkout UI extension point", analysis)
		}
	})

	t.Run("Magento Japan tax", func(t *testing.T) {
		analysis := analyzeRealMagentoFixture(t, "../../pipeline/fixtures/real-modules/magento-japan-tax/magento-japan-tax-c0e781438f8f46b65f37d30882199c6717a08d12")
		if analysis.ModuleName != "Magentoj_JapaneseConsumptionTax" || len(analysis.Events) == 0 || len(analysis.Plugins) == 0 {
			t.Fatalf("analysis = %#v, want module metadata, events and plugins", analysis)
		}
	})

	t.Run("Magento gRPC", func(t *testing.T) {
		analysis := analyzeRealMagentoFixture(t, "../../pipeline/fixtures/real-modules/magento-module-grpc/module-grpc-6fe9d46de08dc5eda06d72053c560cd9207f1707")
		if analysis.ModuleName != "Magento_Grpc" || len(analysis.Arguments) == 0 {
			t.Fatalf("analysis = %#v, want module metadata and DI arguments", analysis)
		}
	})
}

func analyzeRealMagentoFixture(t *testing.T, moduleDir string) *MagentoAnalysis {
	t.Helper()
	base := filepath.Join(moduleDir, "etc")
	returnValue, err := AnalyzeMagentoModule(
		readProjectFixture(t, filepath.Join(base, "module.xml")),
		readProjectFixtureOptional(t, filepath.Join(base, "events.xml")),
		readProjectFixtureOptional(t, filepath.Join(base, "di.xml")),
	)
	if err != nil {
		t.Fatalf("AnalyzeMagentoModule() returned error: %v", err)
	}
	return returnValue
}

func readProjectFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read project fixture %q: %v", path, err)
	}
	return data
}

func readProjectFixtureOptional(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read project fixture %q: %v", path, err)
	}
	return data
}
