//go:build wasmtoolchain

package wasm

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"euromni/internal/abi"
)

func TestRustSelfnamedPreviewGuestMapsProductInGuest(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	guestDirectory := filepath.Join(projectRoot, "pipeline", "testdata", "rust-selfnamed-preview")
	targetDirectory := filepath.Join(t.TempDir(), "target")
	command := exec.Command(
		"cargo",
		"build",
		"--frozen",
		"--release",
		"--target", "wasm32-unknown-unknown",
		"--target-dir", targetDirectory,
	)
	command.Dir = guestDirectory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Rust Selfnamed guest: %v\n%s", err, output)
	}
	guestPath := filepath.Join(targetDirectory, "wasm32-unknown-unknown", "release", "euromni_selfnamed_preview_guest.wasm")
	guest, err := os.ReadFile(guestPath)
	if err != nil {
		t.Fatalf("read Rust Selfnamed guest: %v", err)
	}

	const moduleName = "rust-selfnamed-preview"
	host := &Host{}
	runtime := NewRuntime(host)
	t.Cleanup(runtime.Close)
	if err := runtime.Load(moduleName, guest); err != nil {
		t.Fatalf("load Rust Selfnamed guest: %v", err)
	}
	tc, err := trustedContext("e0e00000-0001-4000-8000-000000000001", "e0e00000-0001-4000-8000-000000000080", CapCatalog)
	if err != nil {
		t.Fatal(err)
	}

	input := []byte(`{"abi":"euromni.abi.v1","call":"catalog.supplier.product.preview","input":{"product_json":"{\"id\":842003,\"name\":\"Synthetic Out-of-Stock Balm\",\"description\":\"Synthetic unavailable stock case.\",\"price\":17.40,\"volume\":88,\"weight\":1001,\"isInStock\":false,\"tags\":[\" synthetic \",\"edge-case\"]}"}}`)
	capture := &hostResponseCapture{}
	if _, err := runtime.runEntry(context.Background(), tc, manualModuleKey(moduleName), "run", input, capture); err != nil {
		t.Fatalf("execute Rust Selfnamed guest: %v", err)
	}
	if capture.calls != 1 {
		t.Fatalf("guest called host %d times, want one result validation call", capture.calls)
	}
	var response struct {
		OK   bool   `json:"ok"`
		Call string `json:"call"`
		Data struct {
			ExternalID string `json:"external_id"`
			Name       string `json:"name"`
			Price      struct {
				Amount string `json:"amount"`
			} `json:"price"`
			Inventory struct {
				Status string `json:"status"`
			} `json:"inventory"`
			Tags []string `json:"tags"`
		} `json:"data"`
	}
	if err := json.Unmarshal(capture.response, &response); err != nil {
		t.Fatalf("decode guest-mapped response: %v", err)
	}
	if !response.OK || response.Call != "catalog.supplier.product.preview.result" ||
		response.Data.ExternalID != "842003" ||
		response.Data.Name != "Synthetic Out-of-Stock Balm" ||
		response.Data.Price.Amount != "17.4" ||
		response.Data.Inventory.Status != "outofstock" ||
		!reflect.DeepEqual(response.Data.Tags, []string{"synthetic", "edge-case"}) {
		t.Fatalf("guest mapping response = %s", capture.response)
	}

	for _, test := range []struct {
		name        string
		productJSON string
	}{
		{name: "malformed product", productJSON: `{"id":`},
		{name: "oversized product", productJSON: strings.Repeat("x", 8<<10+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := json.Marshal(map[string]any{
				"abi":   abi.Version,
				"call":  "catalog.supplier.product.preview",
				"input": map[string]string{"product_json": test.productJSON},
			})
			if err != nil {
				t.Fatalf("encode invalid product input: %v", err)
			}
			capture := &hostResponseCapture{}
			if _, err := runtime.runEntry(context.Background(), tc, manualModuleKey(moduleName), "run", input, capture); err != nil {
				t.Fatalf("execute Rust guest with invalid product: %v", err)
			}
			var response struct {
				OK    bool           `json:"ok"`
				Error *abi.ErrorBody `json:"error"`
			}
			if err := json.Unmarshal(capture.response, &response); err != nil {
				t.Fatalf("decode invalid product response: %v", err)
			}
			if capture.calls != 1 || response.OK || response.Error == nil || response.Error.Code != abi.CodeInvalid {
				t.Fatalf("invalid product response = %s, want ABI invalid error", capture.response)
			}
		})
	}
}
