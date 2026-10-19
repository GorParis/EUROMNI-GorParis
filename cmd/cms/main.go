package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"euromni/internal/api"
	"euromni/internal/payment"
	"euromni/internal/store"
	"euromni/internal/supplier/selfnamed"
	"euromni/internal/wasm"
)

func main() {
	if err := configureDevelopmentPaymentProvider(); err != nil {
		log.Fatal(err)
	}
	if err := configureSelfnamedSupplierProvider(); err != nil {
		log.Fatal(err)
	}
	reader, closeReader, err := configuredProductReader()
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := closeReader(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()
	closePolicy, err := configureShippingPricingPolicy(reader)
	if err != nil {
		log.Fatal(err)
	}
	defer closePolicy()

	addr := listenAddress()
	mux := newHandler(reader)
	log.Printf("euromni cms listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func configureSelfnamedSupplierProvider() error {
	api.SetSelfnamedOrderProcessor(nil)
	provider := strings.TrimSpace(os.Getenv("EUROMNI_SUPPLIER_PROVIDER"))
	if provider == "" {
		return nil
	}
	if provider != "selfnamed-mock" {
		return fmt.Errorf("unsupported EUROMNI_SUPPLIER_PROVIDER %q", provider)
	}
	if strings.TrimSpace(os.Getenv("EUROMNI_ENVIRONMENT")) != "development" {
		return fmt.Errorf("EUROMNI_SUPPLIER_PROVIDER=selfnamed-mock requires EUROMNI_ENVIRONMENT=development")
	}
	api.SetSelfnamedOrderProcessor(selfnamed.Mock{})
	return nil
}

func configureDevelopmentPaymentProvider() error {
	api.SetDevelopmentPaymentProcessor(nil)
	provider := strings.TrimSpace(os.Getenv("EUROMNI_PAYMENT_PROVIDER"))
	if provider == "" {
		return nil
	}
	if provider != "development-fake" {
		return fmt.Errorf("unsupported EUROMNI_PAYMENT_PROVIDER %q", provider)
	}
	if strings.TrimSpace(os.Getenv("EUROMNI_ENVIRONMENT")) != "development" {
		return fmt.Errorf("EUROMNI_PAYMENT_PROVIDER=development-fake requires EUROMNI_ENVIRONMENT=development")
	}
	api.SetDevelopmentPaymentProcessor(payment.DevelopmentFake{})
	return nil
}

func configureShippingPricingPolicy(reader store.ProductReader) (func(), error) {
	name := strings.TrimSpace(os.Getenv("EUROMNI_SHIPPING_WASM_POLICY"))
	if name == "" {
		return func() {}, nil
	}
	if name != "free-over-100" {
		return nil, fmt.Errorf("unsupported EUROMNI_SHIPPING_WASM_POLICY %q", name)
	}
	setter, ok := reader.(store.ShippingPricingPolicySetter)
	if !ok {
		return nil, fmt.Errorf("configured store does not support shipping pricing policies")
	}
	policy, err := wasm.NewBundledFreeShippingPolicy()
	if err != nil {
		return nil, err
	}
	setter.SetShippingPricingPolicy(policy)
	return policy.Close, nil
}

func configuredProductReader() (store.ProductReader, func() error, error) {
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		return store.NewMemory(), func() error { return nil }, nil
	}
	database, err := store.Open(dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("connect to DATABASE_URL: %w", err)
	}
	return database, database.SQL.Close, nil
}

func listenAddress() string {
	if address := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); address != "" {
		return address
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8081"
	}
	return "127.0.0.1:" + port
}

func newHandler(reader store.ProductReader) http.Handler {
	api.SetProductReader(reader)
	mux := http.NewServeMux()
	api.Register(mux)
	return mux
}
