package wasm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"euromni/internal/abi"
)

const maxSupplierProductJSONBytes = 8 << 10

var supplierDecimalPattern = regexp.MustCompile(`^[0-9]{1,12}(?:\.[0-9]{1,4})?$`)

type supplierProductPreviewInput struct {
	ProductJSON string `json:"product_json"`
}

type selfnamedProduct struct {
	ID          json.Number `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Price       json.Number `json:"price"`
	Volume      int64       `json:"volume"`
	Weight      int64       `json:"weight"`
	IsInStock   *bool       `json:"isInStock"`
	Tags        []string    `json:"tags"`
}

type supplierProductPreview struct {
	Provider          string                          `json:"provider"`
	ExternalID        string                          `json:"external_id"`
	PublicationStatus string                          `json:"publication_status"`
	Name              string                          `json:"name"`
	Description       string                          `json:"description"`
	Locale            *string                         `json:"locale"`
	Price             supplierProductPricePreview     `json:"price"`
	Inventory         supplierProductInventoryPreview `json:"inventory"`
	Weight            supplierProductWeightPreview    `json:"weight"`
	Volume            supplierProductVolumePreview    `json:"volume"`
	Tags              []string                        `json:"tags"`
	Unresolved        []string                        `json:"unresolved"`
}

type supplierProductPricePreview struct {
	Amount   string  `json:"amount"`
	Currency *string `json:"currency"`
}

type supplierProductInventoryPreview struct {
	Status string `json:"status"`
	Track  bool   `json:"track_quantity"`
}

type supplierProductWeightPreview struct {
	Grams int64   `json:"grams"`
	Unit  *string `json:"unit"`
}

type supplierProductVolumePreview struct {
	Value int64   `json:"value"`
	Unit  *string `json:"unit"`
}

func catalogSupplierProductPreview(env *abi.Envelope) (any, error) {
	input, err := abi.DecodeInput[supplierProductPreviewInput](env.Input)
	if err != nil {
		return nil, err
	}
	preview, err := buildSelfnamedProductPreview(input.ProductJSON)
	if err != nil {
		return nil, abi.Fail(abi.CodeInvalid, "invalid supplier product")
	}
	return preview, nil
}

func buildSelfnamedProductPreview(productJSON string) (supplierProductPreview, error) {
	if len(productJSON) == 0 || len(productJSON) > maxSupplierProductJSONBytes || !utf8.ValidString(productJSON) {
		return supplierProductPreview{}, errors.New("product JSON size is invalid")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(productJSON))
	decoder.UseNumber()
	var product selfnamedProduct
	if err := decoder.Decode(&product); err != nil {
		return supplierProductPreview{}, fmt.Errorf("decode supplier product: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return supplierProductPreview{}, errors.New("supplier product has trailing JSON")
	}

	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(productJSON), &fields); err != nil {
		return supplierProductPreview{}, fmt.Errorf("decode supplier product fields: %w", err)
	}
	for _, required := range []string{"id", "name", "price", "volume", "weight", "isInStock"} {
		if _, ok := fields[required]; !ok {
			return supplierProductPreview{}, fmt.Errorf("supplier product is missing %s", required)
		}
	}
	for _, numeric := range []string{"id", "price"} {
		if !isJSONNumber(fields[numeric]) {
			return supplierProductPreview{}, fmt.Errorf("supplier product %s is not a JSON number", numeric)
		}
	}
	externalID := product.ID.String()
	if externalID == "" || len(externalID) > 20 || !isPositiveInteger(externalID) {
		return supplierProductPreview{}, errors.New("supplier product id is invalid")
	}
	name := strings.TrimSpace(product.Name)
	if name == "" || len(name) > 512 || !utf8.ValidString(name) {
		return supplierProductPreview{}, errors.New("supplier product name is invalid")
	}
	if len(product.Description) > 8192 || !utf8.ValidString(product.Description) {
		return supplierProductPreview{}, errors.New("supplier product description is invalid")
	}
	price := product.Price.String()
	if !supplierDecimalPattern.MatchString(price) {
		return supplierProductPreview{}, errors.New("supplier product price is invalid")
	}
	price = normalizeSupplierDecimal(price)
	if product.Weight < 0 || product.Weight > 100_000_000 {
		return supplierProductPreview{}, errors.New("supplier product weight is invalid")
	}
	if product.Volume < 0 || product.Volume > 1_000_000_000 {
		return supplierProductPreview{}, errors.New("supplier product volume is invalid")
	}
	if product.IsInStock == nil {
		return supplierProductPreview{}, errors.New("supplier product stock status is missing")
	}
	if len(product.Tags) > 64 {
		return supplierProductPreview{}, errors.New("supplier product has too many tags")
	}
	tags := make([]string, 0, len(product.Tags))
	for _, tag := range product.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len(tag) > 128 || !utf8.ValidString(tag) {
			return supplierProductPreview{}, errors.New("supplier product tag is invalid")
		}
		tags = append(tags, tag)
	}

	stockStatus := "outofstock"
	if *product.IsInStock {
		stockStatus = "instock"
	}
	return supplierProductPreview{
		Provider:          "selfnamed",
		ExternalID:        externalID,
		PublicationStatus: "draft",
		Name:              name,
		Description:       product.Description,
		Locale:            nil,
		Price: supplierProductPricePreview{
			Amount:   price,
			Currency: nil,
		},
		Inventory: supplierProductInventoryPreview{
			Status: stockStatus,
			Track:  false,
		},
		Weight: supplierProductWeightPreview{Grams: product.Weight, Unit: nil},
		Volume: supplierProductVolumePreview{Value: product.Volume, Unit: nil},
		Tags:   tags,
		Unresolved: []string{
			"currency",
			"locale",
			"volume_unit",
		},
	}, nil
}

func isJSONNumber(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	_, ok := value.(json.Number)
	return ok
}

func isPositiveInteger(value string) bool {
	if value == "" {
		return false
	}
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	value = strings.TrimLeft(value, "0")
	return value != ""
}

func normalizeSupplierDecimal(value string) string {
	if !strings.Contains(value, ".") {
		return value
	}
	return strings.TrimSuffix(strings.TrimRight(value, "0"), ".")
}
