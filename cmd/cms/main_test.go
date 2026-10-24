package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"euromni/internal/api"
	"euromni/internal/dbdialect"
	"euromni/internal/store"
)

func TestNewHandlerWiresMemoryCatalogAndStorefront(t *testing.T) {
	handler := newHandler(store.NewMemory())

	productResponse := httptest.NewRecorder()
	handler.ServeHTTP(productResponse, httptest.NewRequest(http.MethodGet, "/v1/products", nil))
	if productResponse.Code != http.StatusOK {
		t.Fatalf("product endpoint status = %d, want %d; body: %s", productResponse.Code, http.StatusOK, productResponse.Body)
	}
	if body := productResponse.Body.String(); !strings.Contains(body, `"slug":"cabas-lin"`) {
		t.Fatalf("product endpoint response does not contain the seeded catalog: %s", body)
	}

	storefrontResponse := httptest.NewRecorder()
	handler.ServeHTTP(storefrontResponse, httptest.NewRequest(http.MethodGet, "/storefront/", nil))
	if storefrontResponse.Code != http.StatusOK {
		t.Fatalf("storefront status = %d, want %d; body: %s", storefrontResponse.Code, http.StatusOK, storefrontResponse.Body)
	}
	if body := storefrontResponse.Body.String(); !strings.Contains(body, "euromni-catalog") {
		t.Fatalf("storefront response does not contain the catalog component: %s", body)
	}
}

func TestConfiguredProductReaderRejectsUnavailableDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "://invalid")
	reader, closeReader, err := configuredProductReader()
	if err == nil {
		if closeReader != nil {
			_ = closeReader()
		}
		t.Fatalf("configuredProductReader() = %T, nil error; want database connection error", reader)
	}
	if reader != nil || closeReader != nil {
		t.Fatalf("configuredProductReader() returned reader=%T and close=%v on error", reader, closeReader != nil)
	}
}

func TestNewProductsWidgetRequiresSQLStoreWhenArtifactsConfigured(t *testing.T) {
	t.Setenv("EUROMNI_ARTIFACT_DIR", t.TempDir())
	if closeWidget, err := configureNewProductsWidget(store.NewMemory()); err == nil {
		closeWidget()
		t.Fatal("configureNewProductsWidget() succeeded without a SQL store")
	}
}

func TestModuleArchiveQuarantineIsOptInAndUsesPrivateStore(t *testing.T) {
	t.Cleanup(func() { api.SetModuleArchiveQuarantine(nil) })
	t.Setenv("EUROMNI_QUARANTINE_DIR", "")
	if err := configureModuleArchiveQuarantine(); err != nil {
		t.Fatalf("disable module archive quarantine: %v", err)
	}

	t.Setenv("EUROMNI_QUARANTINE_DIR", filepath.Join(t.TempDir(), "quarantine"))
	if err := configureModuleArchiveQuarantine(); err != nil {
		t.Fatalf("configure module archive quarantine: %v", err)
	}
}

func TestTrustedBuildPackageKeysLoadFromExplicitFile(t *testing.T) {
	t.Cleanup(func() { api.SetTrustedBuildPackageKeys(nil) })
	t.Setenv("EUROMNI_BUILD_TRUST_FILE", "")
	if err := configureTrustedBuildPackageKeys(); err != nil {
		t.Fatalf("disable trusted build keys: %v", err)
	}
	if keys := api.TrustedBuildPackageKeys(); len(keys) != 0 {
		t.Fatalf("trusted key count without configuration = %d, want 0", len(keys))
	}

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyID := fmt.Sprintf("%x", sha256.Sum256(publicKey))
	document := fmt.Sprintf(
		`{"version":"euromni.trusted-build-keys.v1","keys":[{"key_id":%q,"public_key":%q}]}`,
		keyID, base64.StdEncoding.EncodeToString(publicKey),
	)
	path := filepath.Join(t.TempDir(), "trusted-keys.json")
	if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EUROMNI_BUILD_TRUST_FILE", path)
	if err := configureTrustedBuildPackageKeys(); err != nil {
		t.Fatalf("configure trusted build keys: %v", err)
	}
	if keys := api.TrustedBuildPackageKeys(); len(keys) != 1 || string(keys[keyID]) != string(publicKey) {
		t.Fatalf("loaded trusted keys = %v, want configured key %q", len(keys), keyID)
	}

	t.Setenv("EUROMNI_BUILD_TRUST_FILE", filepath.Join(t.TempDir(), "missing.json"))
	if err := configureTrustedBuildPackageKeys(); err == nil {
		t.Fatal("configureTrustedBuildPackageKeys() accepted a missing trust file")
	}
	if keys := api.TrustedBuildPackageKeys(); len(keys) != 0 {
		t.Fatalf("trusted key count after invalid configuration = %d, want 0", len(keys))
	}
}

func TestBuildPackageCandidateStorageIsOptIn(t *testing.T) {
	t.Cleanup(func() { api.SetModuleBuildPackageCandidates(nil) })
	t.Setenv("EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR", "")
	if err := configureBuildPackageCandidates(store.NewMemory()); err != nil {
		t.Fatalf("disable build package candidate storage: %v", err)
	}
	t.Setenv("EUROMNI_BUILD_PACKAGE_CANDIDATE_DIR", filepath.Join(t.TempDir(), "candidates"))
	if err := configureBuildPackageCandidates(store.NewMemory()); err == nil ||
		!strings.Contains(err.Error(), "configured PostgreSQL database") {
		t.Fatalf("configure build package candidate storage without PostgreSQL error = %v", err)
	}
	if err := configureBuildPackageCandidates(store.NewSQLWithDialect(nil, dbdialect.PostgreSQL)); err == nil ||
		!strings.Contains(err.Error(), "configured PostgreSQL database") {
		t.Fatalf("configure build package candidate storage without SQL connection error = %v", err)
	}
}

func TestListenAddressDefaultsToLoopbackAndAcceptsOverride(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("PORT", "")
	if got := listenAddress(); got != "127.0.0.1:8081" {
		t.Fatalf("default listen address = %q, want 127.0.0.1:8081", got)
	}
	t.Setenv("PORT", "9090")
	if got := listenAddress(); got != "127.0.0.1:9090" {
		t.Fatalf("port-based listen address = %q, want 127.0.0.1:9090", got)
	}
	t.Setenv("LISTEN_ADDR", "0.0.0.0:8081")
	if got := listenAddress(); got != "0.0.0.0:8081" {
		t.Fatalf("explicit listen address = %q, want 0.0.0.0:8081", got)
	}
}

func TestHTTPServerUsesFiniteTimeouts(t *testing.T) {
	handler := http.NotFoundHandler()
	server := newHTTPServer("127.0.0.1:8081", handler)
	if server.Addr != "127.0.0.1:8081" || server.Handler == nil {
		t.Fatalf("server address/handler = %q/%T", server.Addr, server.Handler)
	}
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("server handler status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 60*time.Second ||
		server.WriteTimeout != 60*time.Second || server.IdleTimeout != 90*time.Second {
		t.Fatalf("server timeouts = header:%s read:%s write:%s idle:%s",
			server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
}

func TestDevelopmentPaymentProviderRequiresExplicitDevelopmentEnvironment(t *testing.T) {
	t.Cleanup(func() { api.SetDevelopmentPaymentProcessor(nil) })
	for _, test := range []struct {
		name        string
		environment string
		provider    string
		wantError   bool
	}{
		{name: "disabled by default", environment: "production"},
		{name: "enabled in development", environment: "development", provider: "development-fake"},
		{name: "blocked outside development", environment: "production", provider: "development-fake", wantError: true},
		{name: "provider validation", environment: "development", provider: "real", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("EUROMNI_ENVIRONMENT", test.environment)
			t.Setenv("EUROMNI_PAYMENT_PROVIDER", test.provider)
			err := configureDevelopmentPaymentProvider()
			if (err != nil) != test.wantError {
				t.Fatalf("configureDevelopmentPaymentProvider() error = %v, want error %t", err, test.wantError)
			}
		})
	}
}

func TestSelfnamedSupplierProviderRequiresExplicitDevelopmentEnvironment(t *testing.T) {
	t.Cleanup(func() { api.SetSelfnamedOrderProcessor(nil) })
	for _, test := range []struct {
		name        string
		environment string
		provider    string
		wantError   bool
	}{
		{name: "disabled by default", environment: "production"},
		{name: "enabled as local mock in development", environment: "development", provider: "selfnamed-mock"},
		{name: "blocked outside development", environment: "production", provider: "selfnamed-mock", wantError: true},
		{name: "provider validation", environment: "development", provider: "selfnamed", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("EUROMNI_ENVIRONMENT", test.environment)
			t.Setenv("EUROMNI_SUPPLIER_PROVIDER", test.provider)
			err := configureSelfnamedSupplierProvider()
			if (err != nil) != test.wantError {
				t.Fatalf("configureSelfnamedSupplierProvider() error = %v, want error %t", err, test.wantError)
			}
		})
	}
}
