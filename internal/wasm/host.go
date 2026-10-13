package wasm

import "euromni/internal/store"

// Capability is the explicit privilege granted to a guest module.
type Capability string

const (
	CapCatalog   Capability = "catalog"
	CapCart      Capability = "cart"
	CapOrder     Capability = "order"
	CapCustomer  Capability = "customer"
	CapPayment   Capability = "payment"
	CapContent   Capability = "content"
	CapHooks     Capability = "hooks"
	CapAdmin     Capability = "admin"
	CapHTTPFetch Capability = "http.fetch"
	CapModuleKV  Capability = "module.kv"
)

// Host is the Go-side sandbox boundary used by guest module calls.
type Host struct {
	Products   store.ProductReader
	Categories store.CategoryReader
}
