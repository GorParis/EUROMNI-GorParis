package wasm

// HostCall is one ABI function the Wasm guest may invoke.
// The host executes SQL; the guest never sees a connection.
type HostCall struct {
	Name   string
	Cap    Capability
	Tables []string
}

// Mapping is euromni.abi.v1 : capability → host calls → tables.
var Mapping = []HostCall{
	{Name: "catalog.product.list", Cap: CapCatalog, Tables: []string{"products", "product_translations", "product_categories"}},
	{Name: "catalog.product.get", Cap: CapCatalog, Tables: []string{"products", "product_translations", "product_media", "media", "variants", "prices", "inventory_items"}},
	{Name: "catalog.product.upsert", Cap: CapCatalog, Tables: []string{"products", "product_translations", "product_categories"}},
	{Name: "catalog.category.list", Cap: CapCatalog, Tables: []string{"categories", "category_translations"}},
	{Name: "catalog.variant.get", Cap: CapCatalog, Tables: []string{"variants", "prices", "inventory_items"}},
	{Name: "catalog.inventory.adjust", Cap: CapCatalog, Tables: []string{"inventory_items"}},

	{Name: "cart.get", Cap: CapCart, Tables: []string{"carts", "cart_lines"}},
	{Name: "cart.add_line", Cap: CapCart, Tables: []string{"carts", "cart_lines", "variants", "prices"}},
	{Name: "cart.update_line", Cap: CapCart, Tables: []string{"cart_lines"}},
	{Name: "cart.clear", Cap: CapCart, Tables: []string{"cart_lines"}},

	{Name: "order.list", Cap: CapOrder, Tables: []string{"orders", "order_lines"}},
	{Name: "order.get", Cap: CapOrder, Tables: []string{"orders", "order_lines", "payments", "shipments"}},
	{Name: "order.place", Cap: CapOrder, Tables: []string{"carts", "cart_lines", "orders", "order_lines", "inventory_items", "domain_events"}},
	{Name: "order.set_status", Cap: CapOrder, Tables: []string{"orders", "domain_events"}},
	{Name: "order.shipment.upsert", Cap: CapOrder, Tables: []string{"shipments"}},

	{Name: "customer.get", Cap: CapCustomer, Tables: []string{"customers"}},
	{Name: "customer.upsert", Cap: CapCustomer, Tables: []string{"customers"}},
	{Name: "customer.address.list", Cap: CapCustomer, Tables: []string{"addresses"}},
	{Name: "customer.consent.set", Cap: CapCustomer, Tables: []string{"customer_consents"}},
	{Name: "session.get", Cap: CapCustomer, Tables: []string{"sessions"}},

	{Name: "payment.initiate", Cap: CapPayment, Tables: []string{"payments", "orders"}},
	{Name: "payment.capture", Cap: CapPayment, Tables: []string{"payments", "orders", "domain_events"}},

	{Name: "content.page.get", Cap: CapContent, Tables: []string{"pages", "page_translations"}},
	{Name: "content.page.upsert", Cap: CapContent, Tables: []string{"pages", "page_translations"}},
	{Name: "content.menu.get", Cap: CapContent, Tables: []string{"menus", "menu_items"}},

	{Name: "hooks.emit", Cap: CapHooks, Tables: []string{"domain_events"}},
	{Name: "hooks.consume", Cap: CapHooks, Tables: []string{"domain_events"}},

	{Name: "admin.shop.get", Cap: CapAdmin, Tables: []string{"shops", "shop_locales"}},
	{Name: "admin.module.config.get", Cap: CapAdmin, Tables: []string{"shop_modules"}},
	{Name: "admin.module.config.set", Cap: CapAdmin, Tables: []string{"shop_modules"}},

	{Name: "http.fetch", Cap: CapHTTPFetch, Tables: []string{}}, // allowlist in Go, not SQL

	{Name: "kv.get", Cap: CapModuleKV, Tables: []string{"module_kv"}},
	{Name: "kv.put", Cap: CapModuleKV, Tables: []string{"module_kv"}},
	{Name: "kv.delete", Cap: CapModuleKV, Tables: []string{"module_kv"}},
}

var callsByName map[string]HostCall

func init() {
	callsByName = make(map[string]HostCall, len(Mapping))
	for _, c := range Mapping {
		callsByName[c.Name] = c
	}
}

func Lookup(name string) (HostCall, bool) {
	c, ok := callsByName[name]
	return c, ok
}
