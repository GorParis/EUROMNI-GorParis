package store

import "testing"

func validSupplierProductDraftInput() SupplierProductDraftInput {
	tracking := false
	weight := int64(1001)
	volume := int64(88)
	return SupplierProductDraftInput{
		Provider:          "selfnamed",
		ExternalID:        "842003",
		PublicationStatus: "draft",
		Name:              "Synthetic balm",
		Description:       "Synthetic product description",
		Price:             SupplierProductPrice{Amount: "17.4"},
		Inventory:         SupplierProductInventory{Status: "outofstock", Track: &tracking},
		Weight:            SupplierProductWeight{Grams: &weight},
		Volume:            SupplierProductVolume{Value: &volume},
		Tags:              []string{"synthetic", "edge-case"},
		Unresolved:        []string{"currency", "locale", "volume_unit"},
	}
}

func TestValidateSupplierProductDraftInput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SupplierProductDraftInput)
		wantOK bool
	}{
		{name: "valid out-of-stock preview", wantOK: true},
		{
			name: "published source product",
			mutate: func(input *SupplierProductDraftInput) {
				input.PublicationStatus = "active"
			},
		},
		{
			name: "unresolved currency",
			mutate: func(input *SupplierProductDraftInput) {
				currency := "EUR"
				input.Price.Currency = &currency
			},
		},
		{
			name: "stock tracking enabled",
			mutate: func(input *SupplierProductDraftInput) {
				tracking := true
				input.Inventory.Track = &tracking
			},
		},
		{
			name: "missing stock tracking flag",
			mutate: func(input *SupplierProductDraftInput) {
				input.Inventory.Track = nil
			},
		},
		{
			name: "missing weight",
			mutate: func(input *SupplierProductDraftInput) {
				input.Weight.Grams = nil
			},
		},
		{
			name: "missing volume",
			mutate: func(input *SupplierProductDraftInput) {
				input.Volume.Value = nil
			},
		},
		{
			name: "missing tags array",
			mutate: func(input *SupplierProductDraftInput) {
				input.Tags = nil
			},
		},
		{
			name: "unsupported unresolved fields",
			mutate: func(input *SupplierProductDraftInput) {
				input.Unresolved = []string{"currency", "locale"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validSupplierProductDraftInput()
			if test.mutate != nil {
				test.mutate(&input)
			}
			err := ValidateSupplierProductDraftInput(input)
			if test.wantOK && err != nil {
				t.Fatalf("ValidateSupplierProductDraftInput() error = %v", err)
			}
			if !test.wantOK && err == nil {
				t.Fatal("ValidateSupplierProductDraftInput() accepted invalid input")
			}
		})
	}
}
