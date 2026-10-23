package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type selfnamedCatalogFixture struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Price       json.Number `json:"price"`
	Volume      int         `json:"volume"`
	Weight      int         `json:"weight"`
	IsInStock   bool        `json:"isInStock"`
	Images      []string    `json:"images"`
	Image       string      `json:"image"`
	Tags        []string    `json:"tags"`
}

type wooProductMappingOracle struct {
	SelfnamedID string
	Status      string
	Name        string
	Description string
	Price       string
	ManageStock bool
	StockStatus string
	Volume      string
	WeightKG    string
}

func TestSelfnamedCatalogProductMappingCharacterization(t *testing.T) {
	fixtureBytes, err := os.ReadFile("../../pipeline/selfnamed-pilot/catalog-products.json")
	if err != nil {
		t.Fatalf("read synthetic Selfnamed catalog fixture: %v", err)
	}

	var sources []selfnamedCatalogFixture
	decoder := json.NewDecoder(bytes.NewReader(fixtureBytes))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&sources); err != nil {
		t.Fatalf("decode synthetic Selfnamed catalog fixture: %v", err)
	}
	if len(sources) != 3 {
		t.Fatalf("fixture cases = %d, want 3", len(sources))
	}
	want := map[int]wooProductMappingOracle{
		842002: {
			SelfnamedID: "842002", Status: "draft", Name: "Synthetic Catalog Balm",
			Description: "Synthetic description used to characterize product mapping.",
			Price:       "28.82", ManageStock: false, StockStatus: "instock", Volume: "176", WeightKG: "0.176",
		},
		842003: {
			SelfnamedID: "842003", Status: "draft", Name: "Synthetic Out-of-Stock Balm",
			Description: "Synthetic unavailable stock case.",
			Price:       "17.4", ManageStock: false, StockStatus: "outofstock", Volume: "88", WeightKG: "1.001",
		},
		842004: {
			SelfnamedID: "842004", Status: "draft", Name: "Synthetic Decimal-Price Balm",
			Description: "",
			Price:       "0.1", ManageStock: false, StockStatus: "instock", Volume: "1", WeightKG: "1",
		},
	}

	seen := make(map[int]struct{}, len(sources))
	for _, source := range sources {
		if _, exists := seen[source.ID]; exists {
			t.Fatalf("duplicate Selfnamed fixture ID %d", source.ID)
		}
		seen[source.ID] = struct{}{}
		got, exists := want[source.ID]
		if !exists {
			t.Fatalf("unexpected Selfnamed fixture ID %d", source.ID)
		}
		if mapped := characterizeWooProductMapping(source); mapped != got {
			t.Errorf("Selfnamed product %d mapping = %#v, want %#v", source.ID, mapped, got)
		}
	}
}

func characterizeWooProductMapping(source selfnamedCatalogFixture) wooProductMappingOracle {
	stockStatus := "outofstock"
	if source.IsInStock {
		stockStatus = "instock"
	}
	return wooProductMappingOracle{
		SelfnamedID: fmt.Sprintf("%d", source.ID),
		Status:      "draft",
		Name:        source.Name,
		Description: source.Description,
		Price:       normalizeSelfnamedDecimal(source.Price.String()),
		ManageStock: false,
		StockStatus: stockStatus,
		Volume:      fmt.Sprintf("%d", source.Volume),
		WeightKG:    normalizeSelfnamedDecimal(fmt.Sprintf("%d.%03d", source.Weight/1000, source.Weight%1000)),
	}
}

func normalizeSelfnamedDecimal(value string) string {
	if !strings.Contains(value, ".") {
		return value
	}
	return strings.TrimSuffix(strings.TrimRight(value, "0"), ".")
}
