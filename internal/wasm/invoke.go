package wasm

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sort"
	"strings"
	"time"

	"euromni/internal/abi"
	"euromni/internal/store"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

const defaultTimeout = 2 * time.Second

// TrustedContext is built by the Go host from shop_modules — never from guest JSON.
type TrustedContext struct {
	shopID   string
	moduleID string
	caps     map[Capability]struct{}
	config   []byte
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
	case "catalog.product.sort":
		return h.catalogProductSort(ctx, tc, env)
	case "catalog.product.newest":
		return h.catalogProductNewest(ctx, tc, env)
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

func (h *Host) catalogProductSort(ctx context.Context, tc TrustedContext, env *abi.Envelope) (any, error) {
	input, err := abi.DecodeInput[struct {
		Locale  string `json:"locale"`
		Orderby string `json:"orderby"`
	}](env.Input)
	if err != nil {
		return nil, err
	}
	if input.Orderby != "alphabetical" && input.Orderby != "reverse_alpha" {
		return nil, abi.Fail(abi.CodeInvalid, "unsupported product sorting option")
	}
	var locale language.Tag
	switch input.Locale {
	case "en-GB":
		locale = language.BritishEnglish
	case "fr-FR":
		locale = language.French
	default:
		return nil, abi.Fail(abi.CodeInvalid, "unsupported product sorting locale")
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
	comparator := collate.New(locale)
	sort.SliceStable(products, func(i, j int) bool {
		leftName := productSortName(products[i], input.Locale)
		rightName := productSortName(products[j], input.Locale)
		comparison := comparator.CompareString(leftName, rightName)
		if comparison == 0 {
			comparison = strings.Compare(products[i].Slug, products[j].Slug)
		}
		if comparison == 0 {
			comparison = strings.Compare(products[i].ID, products[j].ID)
		}
		if input.Orderby == "reverse_alpha" {
			return comparison > 0
		}
		return comparison < 0
	})
	return products, nil
}

func productSortName(product store.ProductView, locale string) string {
	for _, translation := range product.Translations {
		if translation.Locale == locale {
			return translation.Name
		}
	}
	return product.Slug
}

func (h *Host) catalogProductNewest(ctx context.Context, tc TrustedContext, env *abi.Envelope) (any, error) {
	if _, err := abi.DecodeInput[struct{}](env.Input); err != nil {
		return nil, err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(tc.config, &config); err != nil || config == nil {
		return nil, abi.Fail(abi.CodeInvalid, "module configuration is invalid")
	}
	days, err := configuredInteger(config, "days")
	if err != nil {
		return nil, abi.Fail(abi.CodeInvalid, "module configuration is missing days")
	}
	limit, err := configuredInteger(config, "limit")
	if err != nil {
		return nil, abi.Fail(abi.CodeInvalid, "module configuration is missing limit")
	}
	if err := store.ValidateNewProductsQuery(days, limit); err != nil {
		return nil, abi.Fail(abi.CodeInvalid, "module configuration has invalid new-products settings")
	}
	if h == nil || h.Products == nil {
		return nil, errors.New("catalog unavailable")
	}
	reader, ok := h.Products.(store.NewProductsReader)
	if !ok {
		return nil, errors.New("new products unavailable")
	}
	products, err := reader.ListNewProducts(ctx, tc.shopID, days, limit)
	if err != nil {
		return nil, err
	}
	if products == nil {
		products = []store.ProductView{}
	}
	return products, nil
}

func configuredInteger(config map[string]json.RawMessage, key string) (int, error) {
	raw, exists := config[key]
	if !exists {
		return 0, errors.New("setting is missing")
	}
	var value *int
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return 0, errors.New("setting must be an integer")
	}
	return *value, nil
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
