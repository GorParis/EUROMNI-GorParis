package frontend

import (
	"io/fs"
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
