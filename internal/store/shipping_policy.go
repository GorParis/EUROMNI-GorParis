package store

import (
	"context"
	"encoding/hex"
	"errors"
	"math/big"
	"sync"
)

const ShippingPolicyContractVersion = "euromni.shipping.v1"

var ErrShippingPolicyRejected = errors.New("shipping policy returned an unauthorized amount")

type ShippingPricingPolicySnapshot struct {
	ModuleName      string `json:"module_name"`
	ModuleVersion   string `json:"module_version"`
	ModuleSHA256    string `json:"module_sha256"`
	ContractVersion string `json:"contract_version"`
}

type ShippingPricingPolicy interface {
	Evaluate(context.Context, string, string) (string, ShippingPricingPolicySnapshot, error)
}

type ShippingPricingPolicySetter interface {
	SetShippingPricingPolicy(ShippingPricingPolicy)
}

func ApplyShippingPricingPolicy(
	ctx context.Context,
	policy ShippingPricingPolicy,
	baseAmount, subtotal string,
) (string, *ShippingPricingPolicySnapshot, error) {
	if policy == nil {
		return baseAmount, nil, nil
	}
	proposed, snapshot, err := policy.Evaluate(ctx, baseAmount, subtotal)
	if err != nil {
		return "", nil, err
	}
	if snapshot.ModuleName == "" || snapshot.ModuleVersion == "" ||
		len(snapshot.ModuleSHA256) != 64 || snapshot.ContractVersion != ShippingPolicyContractVersion {
		return "", nil, errors.New("shipping pricing module returned invalid identity metadata")
	}
	if _, err := hex.DecodeString(snapshot.ModuleSHA256); err != nil {
		return "", nil, errors.New("shipping pricing module returned invalid hash metadata")
	}
	base, ok := new(big.Rat).SetString(baseAmount)
	if !ok || base.Sign() < 0 {
		return "", nil, errors.New("invalid base shipping amount")
	}
	subtotalAmount, ok := new(big.Rat).SetString(subtotal)
	if !ok || subtotalAmount.Sign() < 0 {
		return "", nil, errors.New("invalid shipping pricing subtotal")
	}
	expected := new(big.Rat).Set(base)
	threshold := big.NewRat(100, 1)
	if subtotalAmount.Cmp(threshold) >= 0 {
		expected.SetInt64(0)
	}
	proposedAmount, ok := new(big.Rat).SetString(proposed)
	if !ok || proposedAmount.Sign() < 0 || proposedAmount.Cmp(expected) != 0 {
		return "", nil, ErrShippingPolicyRejected
	}
	return expected.FloatString(4), &snapshot, nil
}

func applyShippingPricingPolicy(
	ctx context.Context,
	policy ShippingPricingPolicy,
	option *ShippingOption,
	subtotal *big.Rat,
) error {
	if option == nil || subtotal == nil {
		return errors.New("shipping policy input is incomplete")
	}
	amount, snapshot, err := ApplyShippingPricingPolicy(
		ctx, policy, option.Amount, subtotal.FloatString(4),
	)
	if err != nil {
		return err
	}
	option.Amount = amount
	option.PolicySnapshot = snapshot
	return nil
}

type shippingPricingPolicyHolder struct {
	mu     sync.RWMutex
	policy ShippingPricingPolicy
}

func (s *Store) SetShippingPricingPolicy(policy ShippingPricingPolicy) {
	s.shippingPricingPolicyHolder.set(policy)
}

func (m *Memory) SetShippingPricingPolicy(policy ShippingPricingPolicy) {
	m.shippingPricingPolicyHolder.set(policy)
}

func (h *shippingPricingPolicyHolder) set(policy ShippingPricingPolicy) {
	h.mu.Lock()
	h.policy = policy
	h.mu.Unlock()
}

func (h *shippingPricingPolicyHolder) get() ShippingPricingPolicy {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.policy
}
