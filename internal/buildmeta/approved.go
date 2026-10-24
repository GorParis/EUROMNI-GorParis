package buildmeta

const (
	ManifestVersion           = "euromni.build.v1"
	IRVersion                 = "euromni.ir.v3"
	LegacyWASMCompiler        = "wasmtime-go/v36.Wat2Wasm"
	PreviousIRWASMCompiler    = "wasmtime-go/v36.0.0.Wat2Wasm+euromni.ir.sha256.v1"
	CurrentWASMCompiler       = "wasmtime-go/v36.0.0.Wat2Wasm+euromni.ir.sha256.v1+euromni.compiler.v1"
	SelfnamedPreviewCompiler  = "rustc/1.99.1+wasm32-unknown-unknown+euromni.ir.sha256.v1"
	PrestaShopAdapter         = "prestashop"
	PrestaShopAdapterVersion  = "1.0.0"
	WooCommerceAdapter        = "woocommerce"
	WooCommerceAdapterVersion = "1.0.0"
)

func IsApprovedCompiler(version string) bool {
	return version == LegacyWASMCompiler ||
		version == PreviousIRWASMCompiler ||
		version == CurrentWASMCompiler ||
		version == SelfnamedPreviewCompiler
}

func IsApprovedAdapter(platform, adapter, version string) bool {
	switch platform {
	case PrestaShopAdapter:
		return adapter == PrestaShopAdapter && version == PrestaShopAdapterVersion
	case WooCommerceAdapter:
		return adapter == WooCommerceAdapter && version == WooCommerceAdapterVersion
	default:
		return false
	}
}
