package wasm

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"euromni/internal/abi"
	"euromni/internal/store"

	"github.com/bytecodealliance/wasmtime-go/v36"
)

//go:embed free_shipping_threshold.wat
var freeShippingThresholdWAT string

const (
	freeShippingModuleName    = "demo.shipping.free-over-100"
	freeShippingModuleVersion = "1.0.0"
	shippingAmountScale       = 10_000
)

type BundledFreeShippingPolicy struct {
	runtime  *Runtime
	snapshot store.ShippingPricingPolicySnapshot
}

func NewBundledFreeShippingPolicy() (*BundledFreeShippingPolicy, error) {
	wasmBytes, err := wasmtime.Wat2Wasm(freeShippingThresholdWAT)
	if err != nil {
		return nil, fmt.Errorf("compile bundled free-shipping policy: %w", err)
	}
	digest := sha256.Sum256(wasmBytes)
	runtime := New()
	if err := runtime.Load(freeShippingModuleName, wasmBytes); err != nil {
		runtime.Close()
		return nil, fmt.Errorf("load bundled free-shipping policy: %w", err)
	}
	return &BundledFreeShippingPolicy{
		runtime: runtime,
		snapshot: store.ShippingPricingPolicySnapshot{
			ModuleName:      freeShippingModuleName,
			ModuleVersion:   freeShippingModuleVersion,
			ModuleSHA256:    hex.EncodeToString(digest[:]),
			ContractVersion: store.ShippingPolicyContractVersion,
		},
	}, nil
}

func (p *BundledFreeShippingPolicy) Evaluate(ctx context.Context, baseAmount, subtotal string) (string, store.ShippingPricingPolicySnapshot, error) {
	if p == nil || p.runtime == nil {
		return "", store.ShippingPricingPolicySnapshot{}, errors.New("bundled shipping policy is not initialized")
	}
	base, err := amountUnits(baseAmount)
	if err != nil {
		return "", store.ShippingPricingPolicySnapshot{}, fmt.Errorf("encode base shipping amount for wasm: %w", err)
	}
	subtotalUnits, err := amountUnits(subtotal)
	if err != nil {
		return "", store.ShippingPricingPolicySnapshot{}, fmt.Errorf("encode shipping subtotal for wasm: %w", err)
	}
	input, err := json.Marshal(struct {
		ABI   string `json:"abi"`
		Call  string `json:"call"`
		Input struct {
			SubtotalUnits int64 `json:"subtotal_units"`
			BaseUnits     int64 `json:"base_units"`
		} `json:"input"`
	}{
		ABI:  abi.Version,
		Call: "shipping.rate.quote",
		Input: struct {
			SubtotalUnits int64 `json:"subtotal_units"`
			BaseUnits     int64 `json:"base_units"`
		}{SubtotalUnits: subtotalUnits, BaseUnits: base},
	})
	if err != nil {
		return "", store.ShippingPricingPolicySnapshot{}, fmt.Errorf("encode shipping policy input: %w", err)
	}
	results, err := p.runtime.runWithInput(ctx, TrustedContext{}, manualModuleKey(freeShippingModuleName), "run", input)
	if err != nil {
		return "", store.ShippingPricingPolicySnapshot{}, fmt.Errorf("run bundled free-shipping policy: %w", err)
	}
	if len(results) != 1 {
		return "", store.ShippingPricingPolicySnapshot{}, errors.New("bundled shipping policy returned an invalid result")
	}
	units, ok := results[0].(int64)
	if !ok || units < 0 {
		return "", store.ShippingPricingPolicySnapshot{}, errors.New("bundled shipping policy returned an invalid amount")
	}
	return new(big.Rat).SetFrac(big.NewInt(units), big.NewInt(shippingAmountScale)).FloatString(4), p.snapshot, nil
}

func (p *BundledFreeShippingPolicy) Close() {
	if p != nil && p.runtime != nil {
		p.runtime.Close()
	}
}

func amountUnits(amount string) (int64, error) {
	value, ok := new(big.Rat).SetString(amount)
	if !ok || value.Sign() < 0 {
		return 0, errors.New("amount must be a non-negative decimal")
	}
	units := new(big.Rat).Mul(value, big.NewRat(shippingAmountScale, 1))
	if !units.IsInt() || !units.Num().IsInt64() {
		return 0, errors.New("amount exceeds the wasm pricing range")
	}
	return units.Num().Int64(), nil
}
