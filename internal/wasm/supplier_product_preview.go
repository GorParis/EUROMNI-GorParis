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

type supplierProductPreviewResultInput struct {
	Preview json.RawMessage `json:"preview"`
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

func catalogSupplierProductPreviewResult(env *abi.Envelope) (any, error) {
	input, err := abi.DecodeInput[supplierProductPreviewResultInput](env.Input)
	if err != nil || len(input.Preview) == 0 {
		return nil, abi.Fail(abi.CodeInvalid, "invalid preview result")
	}
	if err := validatePreviewObjectFields(input.Preview); err != nil {
		return nil, abi.Fail(abi.CodeInvalid, "invalid preview result")
	}
	decoder := json.NewDecoder(bytes.NewReader(input.Preview))
	decoder.DisallowUnknownFields()
	var preview supplierProductPreview
	if err := decoder.Decode(&preview); err != nil {
		return nil, abi.Fail(abi.CodeInvalid, "invalid preview result")
	}
	if err := validateSupplierProductPreview(preview); err != nil {
		return nil, abi.Fail(abi.CodeInvalid, "invalid preview result")
	}
	return preview, nil
}

func validatePreviewObjectFields(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errors.New("preview result must be an object")
	}
	for _, field := range []string{
		"provider", "external_id", "publication_status", "name", "description",
		"locale", "price", "inventory", "weight", "volume", "tags", "unresolved",
	} {
		if _, exists := fields[field]; !exists {
			return fmt.Errorf("preview result is missing %s", field)
		}
	}
	if !isJSONArray(fields["tags"]) || !isJSONArray(fields["unresolved"]) {
		return errors.New("preview result tags and unresolved fields must be arrays")
	}
	for objectName, required := range map[string][]string{
		"price":     {"amount", "currency"},
		"inventory": {"status", "track_quantity"},
		"weight":    {"grams", "unit"},
		"volume":    {"value", "unit"},
	} {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(fields[objectName], &object); err != nil || object == nil {
			return fmt.Errorf("preview result %s must be an object", objectName)
		}
		for _, field := range required {
			if _, exists := object[field]; !exists {
				return fmt.Errorf("preview result %s is missing %s", objectName, field)
			}
		}
		switch objectName {
		case "inventory":
			if !isJSONBoolean(object["track_quantity"]) {
				return errors.New("preview result track_quantity must be a boolean")
			}
		case "weight":
			if !isJSONInteger(object["grams"]) {
				return errors.New("preview result grams must be an integer")
			}
		case "volume":
			if !isJSONInteger(object["value"]) {
				return errors.New("preview result volume must be an integer")
			}
		}
	}
	return nil
}

func isJSONArray(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && raw[0] == '['
}

func isJSONInteger(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var value int64
	return json.Unmarshal(raw, &value) == nil
}

func isJSONBoolean(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false"))
}

func validateSupplierProductPreview(preview supplierProductPreview) error {
	if preview.Provider != "selfnamed" ||
		preview.PublicationStatus != "draft" ||
		preview.Name == "" || strings.TrimSpace(preview.Name) != preview.Name ||
		len(preview.Name) > 512 || !utf8.ValidString(preview.Name) ||
		len(preview.Description) > 8192 || !utf8.ValidString(preview.Description) {
		return errors.New("preview result identity or text is invalid")
	}
	if preview.ExternalID == "" || len(preview.ExternalID) > 20 || !isPositiveInteger(preview.ExternalID) {
		return errors.New("preview result external ID is invalid")
	}
	if preview.Locale != nil || !supplierDecimalPattern.MatchString(preview.Price.Amount) ||
		normalizeSupplierDecimal(preview.Price.Amount) != preview.Price.Amount ||
		preview.Price.Currency != nil {
		return errors.New("preview result price or locale is invalid")
	}
	if (preview.Inventory.Status != "instock" && preview.Inventory.Status != "outofstock") ||
		preview.Inventory.Track {
		return errors.New("preview result inventory is invalid")
	}
	if preview.Weight.Grams < 0 || preview.Weight.Grams > 100_000_000 || preview.Weight.Unit != nil ||
		preview.Volume.Value < 0 || preview.Volume.Value > 1_000_000_000 || preview.Volume.Unit != nil {
		return errors.New("preview result dimensions are invalid")
	}
	if preview.Tags == nil || len(preview.Tags) > 64 {
		return errors.New("preview result has too many tags")
	}
	for _, tag := range preview.Tags {
		if tag == "" || len(tag) > 128 || strings.TrimSpace(tag) != tag || !utf8.ValidString(tag) {
			return errors.New("preview result tag is invalid")
		}
	}
	if len(preview.Unresolved) != 3 ||
		preview.Unresolved[0] != "currency" ||
		preview.Unresolved[1] != "locale" ||
		preview.Unresolved[2] != "volume_unit" {
		return errors.New("preview result unresolved fields are invalid")
	}
	return nil
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
