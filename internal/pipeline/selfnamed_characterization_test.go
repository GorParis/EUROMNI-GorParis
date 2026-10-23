package pipeline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
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
	fixtureBytes, err := os.ReadFile("../../pipeline/selfnamed-pilot/catalog-product.json")
	if err != nil {
		t.Fatalf("read synthetic Selfnamed catalog fixture: %v", err)
	}

	var source selfnamedCatalogFixture
	decoder := json.NewDecoder(bytes.NewReader(fixtureBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&source); err != nil {
		t.Fatalf("decode synthetic Selfnamed catalog fixture: %v", err)
	}

	got := characterizeWooProductMapping(source)
	want := wooProductMappingOracle{
		SelfnamedID: "842002",
		Status:      "draft",
		Name:        "Synthetic Catalog Balm",
		Description: "Synthetic description used to characterize product mapping.",
		Price:       "28.82",
		ManageStock: false,
		StockStatus: "instock",
		Volume:      "176",
		WeightKG:    "0.176",
	}
	if got != want {
		t.Fatalf("Selfnamed product mapping = %#v, want %#v", got, want)
	}

	source.IsInStock = false
	if got := characterizeWooProductMapping(source).StockStatus; got != "outofstock" {
		t.Fatalf("out-of-stock mapping = %q, want outofstock", got)
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
		Price:       source.Price.String(),
		ManageStock: false,
		StockStatus: stockStatus,
		Volume:      fmt.Sprintf("%d", source.Volume),
		WeightKG:    fmt.Sprintf("%d.%03d", source.Weight/1000, source.Weight%1000),
	}
}
