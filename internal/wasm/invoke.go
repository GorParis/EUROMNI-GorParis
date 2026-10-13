package wasm

import (
	"context"
	"errors"
	"log"
	"time"

	"euromni/internal/abi"
	"euromni/internal/store"
)

const defaultTimeout = 2 * time.Second

// TrustedContext is built by the Go host from shop_modules — never from guest JSON.
type TrustedContext struct {
	shopID   string
	moduleID string
	caps     map[Capability]struct{}
}

func trustedContext(shopID, moduleID string, caps ...Capability) (TrustedContext, error) {
	sid, err := abi.RequireUUID("shop_id", shopID)
	if err != nil {
		return TrustedContext{}, err
	}
	mid, err := abi.RequireUUID("module_id", moduleID)
	if err != nil {
		return TrustedContext{}, err
	}
	set := make(map[Capability]struct{}, len(caps))
	for _, c := range caps {
		set[c] = struct{}{}
	}
	return TrustedContext{shopID: sid, moduleID: mid, caps: set}, nil
}

func (c TrustedContext) Has(cap Capability) bool {
	_, ok := c.caps[cap]
	return ok
}

type productGetInput struct {
	ProductID string `json:"product_id"`
}

func (h *Host) Invoke(ctx context.Context, tc TrustedContext, raw []byte) []byte {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
		defer cancel()
	}

	env, err := abi.ParseEnvelope(raw)
	if err != nil {
		code, msg := abi.GuestCode(err)
		return abi.EncodeErr("", "", code, msg)
	}

	spec, ok := Lookup(env.Call)
	if !ok {
		return abi.EncodeErr(env.Call, env.ID, abi.CodeInvalid, "unknown call")
	}
	if !tc.Has(spec.Cap) {
		return abi.EncodeErr(env.Call, env.ID, abi.CodeDenied, "capability missing")
	}
	if tc.shopID == "" || tc.moduleID == "" {
		return abi.EncodeErr(env.Call, env.ID, abi.CodeDenied, "capability missing")
	}

	data, err := h.dispatch(ctx, tc, env)
	if err != nil {
		code, msg := abi.GuestCode(err)
		if code == abi.CodeInternal {
			log.Printf("abi %s: host error", env.Call)
		}
		return abi.EncodeErr(env.Call, env.ID, code, msg)
	}
	return abi.EncodeOK(env.Call, env.ID, data)
}

func (h *Host) dispatch(ctx context.Context, tc TrustedContext, env *abi.Envelope) (any, error) {
	switch env.Call {
	case "catalog.product.list":
		return h.catalogProductList(ctx, tc, env)
	case "catalog.product.get":
		return h.catalogProductGet(ctx, tc, env)
	case "catalog.category.list":
		return h.catalogCategoryList(ctx, tc, env)
	default:
		return nil, abi.Fail(abi.CodeUnimplemented, "call not implemented")
	}
}

func (h *Host) catalogProductList(ctx context.Context, tc TrustedContext, env *abi.Envelope) (any, error) {
	if _, err := abi.DecodeInput[struct{}](env.Input); err != nil {
		return nil, err
	}
	if h == nil || h.Products == nil {
		return nil, errors.New("catalog unavailable")
	}
	products, err := h.Products.ListProducts(ctx, tc.shopID)
	if err != nil {
		return nil, err
	}
	if products == nil {
		products = []store.ProductView{}
	}
	return products, nil
}

func (h *Host) catalogProductGet(ctx context.Context, tc TrustedContext, env *abi.Envelope) (any, error) {
	in, err := abi.DecodeInput[productGetInput](env.Input)
	if err != nil {
		return nil, err
	}
	productID, err := abi.RequireUUID("product_id", in.ProductID)
	if err != nil {
		return nil, err
	}
	if h == nil || h.Products == nil {
		return nil, errors.New("catalog unavailable")
	}
	p, err := h.Products.GetProduct(ctx, tc.shopID, productID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, abi.Fail(abi.CodeNotFound, "product not found")
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (h *Host) catalogCategoryList(ctx context.Context, tc TrustedContext, env *abi.Envelope) (any, error) {
	if _, err := abi.DecodeInput[struct{}](env.Input); err != nil {
		return nil, err
	}
	if h == nil || h.Categories == nil {
		return nil, errors.New("catalog unavailable")
	}
	categories, err := h.Categories.ListCategories(ctx, tc.shopID)
	if err != nil {
		return nil, err
	}
	if categories == nil {
		categories = []store.CategoryView{}
	}
	return categories, nil
}
