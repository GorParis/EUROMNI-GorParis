package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	gql "github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"

	"euromni/internal/seed"
	"euromni/internal/store"
)

const maxGraphQLRequestBytes = 1 << 20

type graphQLRequest struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

var (
	graphQLSchemaOnce sync.Once
	graphQLSchema     gql.Schema
	graphQLSchemaErr  error
)

type graphQLAuthorizationKey struct{}

func graphql(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	request, err := decodeGraphQLRequest(w, r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeGraphQLError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeGraphQLError(w, http.StatusBadRequest, "invalid GraphQL request")
		return
	}
	if r.Method == http.MethodGet {
		isMutation, err := graphQLRequestIsMutation(request.Query)
		if err == nil && isMutation {
			writeGraphQLError(w, http.StatusMethodNotAllowed, "mutations require POST")
			return
		}
	}
	if strings.TrimSpace(request.Query) == "" {
		writeGraphQLError(w, http.StatusBadRequest, "query is required")
		return
	}

	schema, err := getGraphQLSchema()
	if err != nil {
		writeGraphQLError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	result := gql.Do(gql.Params{
		Schema:         schema,
		RequestString:  request.Query,
		OperationName:  request.OperationName,
		VariableValues: request.Variables,
		Context:        context.WithValue(r.Context(), graphQLAuthorizationKey{}, r.Header.Get("Authorization")),
	})
	writeJSON(w, http.StatusOK, result)
}

func graphQLRequestIsMutation(query string) (bool, error) {
	document, err := parser.Parse(parser.ParseParams{Source: query})
	if err != nil {
		return false, err
	}
	for _, definition := range document.Definitions {
		operation, ok := definition.(*ast.OperationDefinition)
		if ok && operation.GetOperation() == "mutation" {
			return true, nil
		}
	}
	return false, nil
}

func decodeGraphQLRequest(w http.ResponseWriter, r *http.Request) (graphQLRequest, error) {
	if r.Method == http.MethodGet {
		if len(r.URL.RawQuery) > maxGraphQLRequestBytes {
			return graphQLRequest{}, &http.MaxBytesError{Limit: maxGraphQLRequestBytes}
		}
		query := r.URL.Query()
		request := graphQLRequest{
			Query:         query.Get("query"),
			OperationName: query.Get("operationName"),
		}
		if variables := query.Get("variables"); variables != "" {
			if err := json.Unmarshal([]byte(variables), &request.Variables); err != nil {
				return graphQLRequest{}, err
			}
		}
		return request, nil
	}
	if r.Method != http.MethodPost {
		return graphQLRequest{}, errors.New("unsupported method")
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxGraphQLRequestBytes)
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	var request graphQLRequest
	if err := decoder.Decode(&request); err != nil {
		return graphQLRequest{}, err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return graphQLRequest{}, errors.New("multiple JSON values")
		}
		return graphQLRequest{}, err
	}
	return request, nil
}

func getGraphQLSchema() (gql.Schema, error) {
	graphQLSchemaOnce.Do(func() {
		translationType := gql.NewObject(gql.ObjectConfig{
			Name: "ProductTranslation",
			Fields: gql.Fields{
				"locale":      &gql.Field{Type: gql.NewNonNull(gql.String)},
				"name":        &gql.Field{Type: gql.NewNonNull(gql.String)},
				"description": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		mediaType := gql.NewObject(gql.ObjectConfig{
			Name: "ProductMedia",
			Fields: gql.Fields{
				"uri":      &gql.Field{Type: gql.NewNonNull(gql.String)},
				"mimeType": &gql.Field{Type: gql.NewNonNull(gql.String)},
				"alt":      &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		priceType := gql.NewObject(gql.ObjectConfig{
			Name: "ProductPrice",
			Fields: gql.Fields{
				"currency":  &gql.Field{Type: gql.NewNonNull(gql.String)},
				"amount":    &gql.Field{Type: gql.NewNonNull(gql.String)},
				"compareAt": &gql.Field{Type: gql.String},
			},
		})
		inventoryType := gql.NewObject(gql.ObjectConfig{
			Name: "Inventory",
			Fields: gql.Fields{
				"quantity":    &gql.Field{Type: gql.NewNonNull(gql.Int)},
				"track":       &gql.Field{Type: gql.NewNonNull(gql.Boolean)},
				"stockStatus": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		volumeType := gql.NewObject(gql.ObjectConfig{
			Name: "ProductVolume",
			Fields: gql.Fields{
				"value": &gql.Field{Type: gql.NewNonNull(gql.Int)},
				"unit":  &gql.Field{Type: gql.String},
			},
		})
		optionType := gql.NewObject(gql.ObjectConfig{
			Name: "VariantOption",
			Fields: gql.Fields{
				"name":  &gql.Field{Type: gql.NewNonNull(gql.String)},
				"value": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		variantType := gql.NewObject(gql.ObjectConfig{
			Name: "ProductVariant",
			Fields: gql.Fields{
				"id":        &gql.Field{Type: gql.NewNonNull(gql.ID)},
				"sku":       &gql.Field{Type: gql.NewNonNull(gql.String)},
				"options":   &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(optionType)))},
				"prices":    &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(priceType)))},
				"inventory": &gql.Field{Type: gql.NewNonNull(inventoryType)},
			},
		})
		brandType := gql.NewObject(gql.ObjectConfig{
			Name: "Brand",
			Fields: gql.Fields{
				"id":   &gql.Field{Type: gql.NewNonNull(gql.ID)},
				"slug": &gql.Field{Type: gql.NewNonNull(gql.String)},
				"name": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		personalizationChoiceType := gql.NewObject(gql.ObjectConfig{
			Name: "PersonalizationChoice",
			Fields: gql.Fields{
				"value": &gql.Field{Type: gql.NewNonNull(gql.String)},
				"label": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		personalizationOptionType := gql.NewObject(gql.ObjectConfig{
			Name: "PersonalizationOption",
			Fields: gql.Fields{
				"code":     &gql.Field{Type: gql.NewNonNull(gql.String)},
				"label":    &gql.Field{Type: gql.NewNonNull(gql.String)},
				"required": &gql.Field{Type: gql.NewNonNull(gql.Boolean)},
				"choices":  &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(personalizationChoiceType)))},
			},
		})
		personalizationSelectionType := gql.NewObject(gql.ObjectConfig{
			Name: "PersonalizationSelection",
			Fields: gql.Fields{
				"code":        &gql.Field{Type: gql.NewNonNull(gql.String)},
				"label":       &gql.Field{Type: gql.NewNonNull(gql.String)},
				"value":       &gql.Field{Type: gql.NewNonNull(gql.String)},
				"choiceLabel": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		personalizationInputType := gql.NewInputObject(gql.InputObjectConfig{
			Name: "PersonalizationInput",
			Fields: gql.InputObjectConfigFieldMap{
				"code":  &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.String)},
				"value": &gql.InputObjectFieldConfig{Type: gql.NewNonNull(gql.String)},
			},
		})
		productType := gql.NewObject(gql.ObjectConfig{
			Name: "Product",
			Fields: gql.Fields{
				"id":                     &gql.Field{Type: gql.NewNonNull(gql.ID)},
				"slug":                   &gql.Field{Type: gql.NewNonNull(gql.String)},
				"status":                 &gql.Field{Type: gql.NewNonNull(gql.String)},
				"volume":                 &gql.Field{Type: volumeType},
				"tags":                   &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(gql.String)))},
				"brand":                  &gql.Field{Type: brandType},
				"personalizationOptions": &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(personalizationOptionType)))},
				"translations":           &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(translationType)))},
				"media":                  &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(mediaType)))},
				"variants":               &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(variantType)))},
			},
		})
		categoryType := gql.NewObject(gql.ObjectConfig{
			Name: "Category",
			Fields: gql.Fields{
				"id":           &gql.Field{Type: gql.NewNonNull(gql.ID)},
				"slug":         &gql.Field{Type: gql.NewNonNull(gql.String)},
				"translations": &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(translationType)))},
			},
		})
		cartItemType := gql.NewObject(gql.ObjectConfig{
			Name: "CartItem",
			Fields: gql.Fields{
				"variantId":        &gql.Field{Type: gql.NewNonNull(gql.ID)},
				"sku":              &gql.Field{Type: gql.NewNonNull(gql.String)},
				"quantity":         &gql.Field{Type: gql.NewNonNull(gql.Int)},
				"unitAmount":       &gql.Field{Type: gql.NewNonNull(gql.String)},
				"personalizations": &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(personalizationSelectionType)))},
			},
		})
		cartTotalType := gql.NewObject(gql.ObjectConfig{
			Name: "CartTotal",
			Fields: gql.Fields{
				"amount":   &gql.Field{Type: gql.NewNonNull(gql.String)},
				"currency": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		cartType := gql.NewObject(gql.ObjectConfig{
			Name: "Cart",
			Fields: gql.Fields{
				"items": &gql.Field{Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(cartItemType)))},
				"total": &gql.Field{Type: gql.NewNonNull(cartTotalType)},
			},
		})
		cartSessionType := gql.NewObject(gql.ObjectConfig{
			Name: "CartSession",
			Fields: gql.Fields{
				"token": &gql.Field{Type: gql.NewNonNull(gql.String)},
			},
		})
		queryType := gql.NewObject(gql.ObjectConfig{
			Name: "Query",
			Fields: gql.Fields{
				"cart": &gql.Field{
					Type: cartType,
					Resolve: func(params gql.ResolveParams) (interface{}, error) {
						reader, tokenHash, err := graphQLCartAccess(params)
						if err != nil {
							return nil, err
						}
						cart, err := reader.GetCart(params.Context, seed.ShopID, tokenHash)
						if err != nil {
							return nil, graphQLCartError(err)
						}
						return cartGraphQLValue(cart), nil
					},
				},
				"categories": &gql.Field{
					Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(categoryType))),
					Resolve: func(params gql.ResolveParams) (interface{}, error) {
						reader, ok := currentProductReader().(store.CategoryReader)
						if !ok {
							return nil, errors.New("internal server error")
						}
						categories, err := reader.ListCategories(params.Context, seed.ShopID)
						if err != nil {
							return nil, errors.New("internal server error")
						}
						values := make([]interface{}, 0, len(categories))
						for i := range categories {
							values = append(values, categoryGraphQLValue(&categories[i]))
						}
						return values, nil
					},
				},
				"category": &gql.Field{
					Type: categoryType,
					Args: gql.FieldConfigArgument{
						"id": &gql.ArgumentConfig{Type: gql.NewNonNull(gql.ID)},
					},
					Resolve: func(params gql.ResolveParams) (interface{}, error) {
						id, ok := params.Args["id"].(string)
						if !ok || id == "" {
							return nil, errors.New("invalid category id")
						}
						reader, ok := currentProductReader().(store.CategoryReader)
						if !ok {
							return nil, errors.New("internal server error")
						}
						category, err := reader.GetCategory(params.Context, seed.ShopID, id)
						if errors.Is(err, store.ErrNotFound) {
							return nil, nil
						}
						if err != nil {
							return nil, errors.New("internal server error")
						}
						return categoryGraphQLValue(category), nil
					},
				},
				"products": &gql.Field{
					Type: gql.NewNonNull(gql.NewList(gql.NewNonNull(productType))),
					Resolve: func(params gql.ResolveParams) (interface{}, error) {
						products, err := currentProductReader().ListProducts(params.Context, seed.ShopID)
						if err != nil {
							return nil, errors.New("internal server error")
						}
						values := make([]interface{}, 0, len(products))
						for i := range products {
							values = append(values, productGraphQLValue(&products[i]))
						}
						return values, nil
					},
				},
				"product": &gql.Field{
					Type: productType,
					Args: gql.FieldConfigArgument{
						"id": &gql.ArgumentConfig{Type: gql.NewNonNull(gql.ID)},
					},
					Resolve: func(params gql.ResolveParams) (interface{}, error) {
						id, ok := params.Args["id"].(string)
						if !ok || id == "" {
							return nil, errors.New("invalid product id")
						}
						product, err := currentProductReader().GetProduct(params.Context, seed.ShopID, id)
						if errors.Is(err, store.ErrNotFound) {
							return nil, nil
						}
						if err != nil {
							return nil, errors.New("internal server error")
						}
						return productGraphQLValue(product), nil
					},
				},
			},
		})
		mutationType := gql.NewObject(gql.ObjectConfig{
			Name: "Mutation",
			Fields: gql.Fields{
				"createCart": &gql.Field{
					Type: gql.NewNonNull(cartSessionType),
					Resolve: func(params gql.ResolveParams) (interface{}, error) {
						reader, ok := currentProductReader().(store.CartReader)
						if !ok {
							return nil, errors.New("cart service unavailable")
						}
						token, err := createCartToken(params.Context, reader)
						if err != nil {
							return nil, errors.New("internal server error")
						}
						return map[string]interface{}{"token": token}, nil
					},
				},
				"addCartLine": &gql.Field{
					Type: cartType,
					Args: gql.FieldConfigArgument{
						"variantId":        &gql.ArgumentConfig{Type: gql.NewNonNull(gql.ID)},
						"quantity":         &gql.ArgumentConfig{Type: gql.NewNonNull(gql.Int)},
						"personalizations": &gql.ArgumentConfig{Type: gql.NewList(gql.NewNonNull(personalizationInputType))},
					},
					Resolve: func(params gql.ResolveParams) (interface{}, error) {
						reader, tokenHash, err := graphQLCartAccess(params)
						if err != nil {
							return nil, err
						}
						variantID, _ := params.Args["variantId"].(string)
						quantity, _ := params.Args["quantity"].(int)
						personalizations, err := graphQLPersonalizationValues(params.Args["personalizations"])
						if err != nil {
							return nil, err
						}
						cart, err := reader.AddCartLine(params.Context, seed.ShopID, tokenHash, variantID, quantity, personalizations)
						if err != nil {
							return nil, graphQLCartError(err)
						}
						return cartGraphQLValue(cart), nil
					},
				},
			},
		})
		graphQLSchema, graphQLSchemaErr = gql.NewSchema(gql.SchemaConfig{Query: queryType, Mutation: mutationType})
	})
	return graphQLSchema, graphQLSchemaErr
}

func productGraphQLValue(product *store.ProductView) map[string]interface{} {
	translations := make([]interface{}, 0, len(product.Translations))
	for _, translation := range product.Translations {
		translations = append(translations, map[string]interface{}{
			"locale":      translation.Locale,
			"name":        translation.Name,
			"description": translation.Description,
		})
	}

	media := make([]interface{}, 0, len(product.Media))
	for _, item := range product.Media {
		media = append(media, map[string]interface{}{
			"uri":      item.URI,
			"mimeType": item.MimeType,
			"alt":      item.Alt,
		})
	}

	variants := make([]interface{}, 0, len(product.Variants))
	for _, variant := range product.Variants {
		stockStatus := variant.Inventory.StockStatus
		if stockStatus == "" {
			stockStatus = "in_stock"
		}
		optionNames := make([]string, 0, len(variant.Options))
		for name := range variant.Options {
			optionNames = append(optionNames, name)
		}
		sort.Strings(optionNames)
		options := make([]interface{}, 0, len(optionNames))
		for _, name := range optionNames {
			options = append(options, map[string]interface{}{
				"name":  name,
				"value": fmt.Sprint(variant.Options[name]),
			})
		}

		prices := make([]interface{}, 0, len(variant.Prices))
		for _, price := range variant.Prices {
			var compareAt interface{}
			if price.CompareAt != nil {
				compareAt = *price.CompareAt
			}
			prices = append(prices, map[string]interface{}{
				"currency":  price.Currency,
				"amount":    price.Amount,
				"compareAt": compareAt,
			})
		}

		variants = append(variants, map[string]interface{}{
			"id":      variant.ID,
			"sku":     variant.SKU,
			"options": options,
			"prices":  prices,
			"inventory": map[string]interface{}{
				"quantity":    variant.Inventory.Quantity,
				"track":       variant.Inventory.Track,
				"stockStatus": stockStatus,
			},
		})
	}
	var volume interface{}
	if product.Volume != nil {
		var unit interface{}
		if product.Volume.Unit != nil {
			unit = *product.Volume.Unit
		}
		volume = map[string]interface{}{"value": product.Volume.Value, "unit": unit}
	}
	var brand interface{}
	if product.Brand != nil {
		brand = map[string]interface{}{
			"id": product.Brand.ID, "slug": product.Brand.Slug, "name": product.Brand.Name,
		}
	}
	personalizationOptions := make([]interface{}, 0, len(product.PersonalizationOptions))
	for _, option := range product.PersonalizationOptions {
		choices := make([]interface{}, 0, len(option.Choices))
		for _, choice := range option.Choices {
			choices = append(choices, map[string]interface{}{"value": choice.Value, "label": choice.Label})
		}
		personalizationOptions = append(personalizationOptions, map[string]interface{}{
			"code": option.Code, "label": option.Label, "required": option.Required, "choices": choices,
		})
	}

	tags := append([]string{}, product.Tags...)
	return map[string]interface{}{
		"id":                     product.ID,
		"slug":                   product.Slug,
		"status":                 product.Status,
		"volume":                 volume,
		"tags":                   tags,
		"brand":                  brand,
		"personalizationOptions": personalizationOptions,
		"translations":           translations,
		"media":                  media,
		"variants":               variants,
	}
}

func graphQLCartAccess(params gql.ResolveParams) (store.CartReader, []byte, error) {
	authorization, _ := params.Context.Value(graphQLAuthorizationKey{}).(string)
	reader, tokenHash, ok := cartReaderFromAuthorization(authorization)
	if !ok {
		return nil, nil, errors.New("cart authorization required")
	}
	return reader, tokenHash, nil
}

func graphQLCartError(err error) error {
	switch {
	case errors.Is(err, store.ErrInvalidCartToken):
		return errors.New("cart authorization required")
	case errors.Is(err, store.ErrInvalidCartQuantity):
		return errors.New("invalid cart quantity")
	case errors.Is(err, store.ErrInvalidPersonalization):
		return errors.New("invalid product personalization")
	case errors.Is(err, store.ErrOutOfStock):
		return errors.New("insufficient inventory")
	case errors.Is(err, store.ErrNotFound):
		return errors.New("cart item not found")
	default:
		return errors.New("internal server error")
	}
}

func cartGraphQLValue(cart *store.CartView) map[string]interface{} {
	items := make([]interface{}, 0, len(cart.Items))
	for _, item := range cart.Items {
		personalizations := make([]interface{}, 0, len(item.Personalizations))
		for _, selection := range item.Personalizations {
			personalizations = append(personalizations, map[string]interface{}{
				"code": selection.Code, "label": selection.Label,
				"value": selection.Value, "choiceLabel": selection.ChoiceLabel,
			})
		}
		items = append(items, map[string]interface{}{
			"variantId":        item.VariantID,
			"sku":              item.SKU,
			"quantity":         item.Quantity,
			"unitAmount":       item.UnitAmount,
			"personalizations": personalizations,
		})
	}
	return map[string]interface{}{
		"items": items,
		"total": map[string]interface{}{
			"amount":   cart.Total.Amount,
			"currency": cart.Total.Currency,
		},
	}
}

func graphQLPersonalizationValues(raw interface{}) (map[string]string, error) {
	values := make(map[string]string)
	if raw == nil {
		return values, nil
	}
	entries, ok := raw.([]interface{})
	if !ok {
		return nil, errors.New("invalid product personalization")
	}
	for _, entry := range entries {
		fields, ok := entry.(map[string]interface{})
		if !ok {
			return nil, errors.New("invalid product personalization")
		}
		code, codeOK := fields["code"].(string)
		value, valueOK := fields["value"].(string)
		if !codeOK || !valueOK || code == "" || value == "" {
			return nil, errors.New("invalid product personalization")
		}
		if _, exists := values[code]; exists {
			return nil, errors.New("duplicate product personalization")
		}
		values[code] = value
	}
	return values, nil
}

func categoryGraphQLValue(category *store.CategoryView) map[string]interface{} {
	translations := make([]interface{}, 0, len(category.Translations))
	for _, translation := range category.Translations {
		translations = append(translations, map[string]interface{}{
			"locale":      translation.Locale,
			"name":        translation.Name,
			"description": translation.Description,
		})
	}
	return map[string]interface{}{
		"id":           category.ID,
		"slug":         category.Slug,
		"translations": translations,
	}
}

func writeGraphQLError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"errors": []map[string]string{{"message": message}},
	})
}
