package wasm

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"euromni/internal/abi"
	"euromni/internal/artifact"
	"euromni/internal/store"
)

type NewProductsWidgetRunner struct {
	Modules   *store.Store
	Artifacts artifact.Store
	Runtime   *Runtime

	mu         sync.Mutex
	loadedHash map[string]string
}

func NewNewProductsWidgetRunner(modules *store.Store, artifacts artifact.Store, runtime *Runtime) (*NewProductsWidgetRunner, error) {
	if modules == nil || modules.SQL == nil || artifacts == nil || runtime == nil {
		return nil, errors.New("module store, artifact store, and Wasm runtime are required")
	}
	return &NewProductsWidgetRunner{
		Modules: modules, Artifacts: artifacts, Runtime: runtime,
		loadedHash: make(map[string]string),
	}, nil
}

func (r *NewProductsWidgetRunner) RunNewProducts(ctx context.Context, shopID string) ([]store.ProductView, bool, error) {
	if r == nil || r.Modules == nil || r.Artifacts == nil || r.Runtime == nil {
		return nil, false, errors.New("new-products module runner is not configured")
	}
	installed, err := r.Modules.GetInstalledModuleByName(ctx, shopID, store.NewProductsModuleName)
	if errors.Is(err, store.ErrNotFound) {
		return []store.ProductView{}, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find installed new-products module: %w", err)
	}
	if len(installed.WASMSHA256) != 32 {
		return nil, false, ErrModuleHashMissing
	}
	digest := hex.EncodeToString(installed.WASMSHA256)
	key := shopID + ":" + installed.ID

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loadedHash[key] != digest {
		wasmBytes, err := r.Artifacts.Get(ctx, digest)
		if err != nil {
			return nil, false, fmt.Errorf("read installed new-products artifact: %w", err)
		}
		if r.loadedHash[key] == "" {
			err = r.Runtime.LoadInstalled(ctx, r.Modules, shopID, installed.ID, wasmBytes)
		} else {
			err = r.Runtime.ReloadInstalled(ctx, r.Modules, shopID, installed.ID, wasmBytes)
		}
		if err != nil {
			return nil, false, fmt.Errorf("load installed new-products module: %w", err)
		}
		r.loadedHash[key] = digest
	}
	responseBytes, err := r.Runtime.RunInstalledWithResponse(ctx, r.Modules, shopID, installed.ID, "run")
	if err != nil {
		return nil, false, fmt.Errorf("execute installed new-products module: %w", err)
	}
	var response struct {
		OK    bool            `json:"ok"`
		ABI   string          `json:"abi"`
		Call  string          `json:"call"`
		Data  json.RawMessage `json:"data"`
		Error *abi.ErrorBody  `json:"error"`
	}
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return nil, false, fmt.Errorf("decode new-products ABI response: %w", err)
	}
	if response.ABI != abi.Version || response.Call != "catalog.product.newest" {
		return nil, false, errors.New("new-products module returned an unexpected ABI response")
	}
	if !response.OK {
		if response.Error == nil {
			return nil, false, errors.New("new-products module returned an invalid ABI error")
		}
		return nil, false, fmt.Errorf("new-products host call failed: %s", response.Error.Code)
	}
	var products []store.ProductView
	if err := json.Unmarshal(response.Data, &products); err != nil {
		return nil, false, fmt.Errorf("decode new-products catalog result: %w", err)
	}
	if products == nil {
		products = []store.ProductView{}
	}
	return products, true, nil
}
