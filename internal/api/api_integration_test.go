//go:build integration

package api

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"euromni/internal/seed"
	"euromni/internal/store"
	"euromni/internal/testdb"
)

func TestPostgresAPIEndToEnd(t *testing.T) {
	db := store.NewSQL(testdb.Start(t))
	shippingConfig, err := db.GetShippingConfig(context.Background(), seed.ShopID)
	if err != nil {
		t.Fatalf("read default shipping configuration: %v", err)
	}
	for index := range shippingConfig.Carriers {
		if shippingConfig.Carriers[index].Code == "colissimo" {
			shippingConfig.Carriers[index].Active = true
		}
	}
	shippingConfig.Services = append(shippingConfig.Services, store.ShippingService{
		CarrierCode: "colissimo", Code: "home", Name: "Domicile", Active: true,
	}, store.ShippingService{
		CarrierCode: "colissimo", Code: "express", Name: "Express", Active: true,
	})
	shippingConfig.Rates = []store.ShippingRate{
		{CarrierCode: "colissimo", ServiceCode: "home", ZoneCode: shippingConfig.Zones[0].Code, Amount: "6.90", Currency: "EUR", Active: true},
		{CarrierCode: "colissimo", ServiceCode: "express", ZoneCode: shippingConfig.Zones[0].Code, Amount: "12.90", Currency: "EUR", Active: true},
	}
	if err := db.ReplaceShippingConfig(context.Background(), seed.ShopID, *shippingConfig); err != nil {
		t.Fatalf("configure shipping for checkout: %v", err)
	}
	originalReader := productReader
	SetProductReader(db)
	t.Cleanup(func() {
		SetProductReader(originalReader)
	})

	mux := http.NewServeMux()
	Register(mux)
	listResponse := performAPIRequest(mux, http.MethodGet, "/v1/products", "", "")
	if listResponse.Code != http.StatusOK || !strings.Contains(listResponse.Body.String(), `"slug":"cabas-lin"`) || !strings.Contains(listResponse.Body.String(), `"slug":"ceinture-cuir"`) {
		t.Fatalf("REST catalog list status/body = %d/%s", listResponse.Code, listResponse.Body)
	}
	productResponse := performAPIRequest(mux, http.MethodGet, "/v1/products/"+seed.ProductID, "", "")
	if productResponse.Code != http.StatusOK {
		t.Fatalf("REST product status = %d; body: %s", productResponse.Code, productResponse.Body)
	}

	categoryResponse := performAPIRequest(mux, http.MethodGet, "/v1/categories", "", "")
	if categoryResponse.Code != http.StatusOK || !strings.Contains(categoryResponse.Body.String(), `"slug":"sacs"`) {
		t.Fatalf("REST categories status/body = %d/%s", categoryResponse.Code, categoryResponse.Body)
	}

	createResponse := performAPIRequest(mux, http.MethodPost, "/v1/cart", "", "")
	if createResponse.Code != http.StatusCreated {
		t.Fatalf("REST cart creation status = %d; body: %s", createResponse.Code, createResponse.Body)
	}
	var created struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatalf("decode cart token: %v", err)
	}
	rawToken, err := base64.RawURLEncoding.DecodeString(created.Data.Token)
	if err != nil {
		t.Fatal("decode generated cart token failed")
	}
	tokenHash := sha256.Sum256(rawToken)
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", tokenHash[:])
	}()

	addBody := `{"variant_id":"` + seed.VariantID + `","quantity":2}`
	addResponse := performAPIRequest(mux, http.MethodPost, "/v1/cart/add", "Bearer "+created.Data.Token, addBody)
	if addResponse.Code != http.StatusOK || !strings.Contains(addResponse.Body.String(), `"amount":"178.0000"`) {
		t.Fatalf("REST cart add status/body = %d/%s", addResponse.Code, addResponse.Body)
	}
	shippingOptions := performAPIRequest(mux, http.MethodGet, "/v1/shipping-options?country=FR", "", "")
	if shippingOptions.Code != http.StatusOK ||
		!strings.Contains(shippingOptions.Body.String(), `"carrier_code":"colissimo"`) ||
		!strings.Contains(shippingOptions.Body.String(), `"amount":"6.9000"`) {
		t.Fatalf("shipping options status/body = %d/%s", shippingOptions.Code, shippingOptions.Body)
	}
	shippingQuote := performAPIRequest(mux, http.MethodPost, "/v1/shipping-quotes",
		"Bearer "+created.Data.Token, `{"country":"FR"}`)
	if shippingQuote.Code != http.StatusOK ||
		!strings.Contains(shippingQuote.Body.String(), `"carrier_code":"colissimo"`) ||
		!strings.Contains(shippingQuote.Body.String(), `"amount":"6.9000"`) {
		t.Fatalf("cart shipping quote status/body = %d/%s", shippingQuote.Code, shippingQuote.Body)
	}
	var shippingQuoteData struct {
		Data []struct {
			QuoteID     string `json:"quote_id"`
			ServiceCode string `json:"service_code"`
			ServiceName string `json:"service_name"`
			RateVersion string `json:"rate_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(shippingQuote.Body.Bytes(), &shippingQuoteData); err != nil {
		t.Fatalf("decode cart shipping quote id: %+v, error = %v", shippingQuoteData, err)
	}
	var homeQuote *struct {
		QuoteID     string `json:"quote_id"`
		ServiceCode string `json:"service_code"`
		ServiceName string `json:"service_name"`
		RateVersion string `json:"rate_version"`
	}
	foundExpress := false
	for index := range shippingQuoteData.Data {
		quote := &shippingQuoteData.Data[index]
		if quote.ServiceCode == "home" {
			homeQuote = quote
		}
		if quote.ServiceCode == "express" && quote.ServiceName == "Express" && quote.RateVersion != "" {
			foundExpress = true
		}
	}
	if len(shippingQuoteData.Data) != 2 || homeQuote == nil || homeQuote.QuoteID == "" ||
		homeQuote.ServiceName != "Domicile" || homeQuote.RateVersion == "" || !foundExpress {
		t.Fatalf("shipping service quotes = %+v, want home and express for one carrier", shippingQuoteData.Data)
	}

	queryBody := `{"query":"{ products { slug } product(id: \"` + seed.ProductID + `\") { slug variants { prices { amount } } } categories { slug } cart { total { amount currency } } }"}`
	graphqlResponse := performAPIRequest(mux, http.MethodPost, "/graphql", "Bearer "+created.Data.Token, queryBody)
	if graphqlResponse.Code != http.StatusOK || !strings.Contains(graphqlResponse.Body.String(), `"amount":"178.0000"`) || !strings.Contains(graphqlResponse.Body.String(), `"slug":"sacs"`) || !strings.Contains(graphqlResponse.Body.String(), `"slug":"ceinture-cuir"`) {
		t.Fatalf("GraphQL status/body = %d/%s", graphqlResponse.Code, graphqlResponse.Body)
	}

	checkoutBody := `{"email":"checkout@example.com","name":"Checkout integration","shipping_carrier_code":"colissimo","shipping_service_code":"home","expected_shipping_total":"6.9000","expected_shipping_quote_id":"` + homeQuote.QuoteID + `","shipping_address":{"line1":"1 rue du Test","city":"Paris","postal_code":"75001","country":"FR"}}`
	checkoutResponse := performAPIRequest(mux, http.MethodPost, "/v1/checkout", "Bearer "+created.Data.Token, checkoutBody)
	if checkoutResponse.Code != http.StatusCreated ||
		!strings.Contains(checkoutResponse.Body.String(), `"shipping_total":"6.9000"`) ||
		!strings.Contains(checkoutResponse.Body.String(), `"grand_total":"184.9000"`) ||
		!strings.Contains(checkoutResponse.Body.String(), `"shipping_carrier_name":"Colissimo"`) ||
		!strings.Contains(checkoutResponse.Body.String(), `"shipping_service_name":"Domicile"`) ||
		!strings.Contains(checkoutResponse.Body.String(), `"shipping_rate_version":"`+homeQuote.RateVersion+`"`) {
		t.Fatalf("REST checkout status/body = %d/%s", checkoutResponse.Code, checkoutResponse.Body)
	}
	var createdOrder struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(checkoutResponse.Body).Decode(&createdOrder); err != nil {
		t.Fatalf("decode checkout order: %v", err)
	}
	if createdOrder.Data.ID == "" {
		t.Fatal("checkout response has no order ID")
	}

	if err := db.BootstrapStaff(context.Background(), seed.ShopID, "owner@atelier-demo.example", "Demo Owner", "integration-password"); err != nil {
		t.Fatalf("bootstrap seeded staff owner: %v", err)
	}
	badLogin := performAPIRequest(mux, http.MethodPost, "/v1/admin/login", "",
		`{"email":"owner@atelier-demo.example","password":"incorrect-password"}`)
	if badLogin.Code != http.StatusUnauthorized {
		t.Fatalf("invalid admin login status/body = %d/%s; want 401", badLogin.Code, badLogin.Body)
	}
	loginResponse := performAPIRequest(mux, http.MethodPost, "/v1/admin/login", "",
		`{"email":"owner@atelier-demo.example","password":"integration-password"}`)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("admin login status/body = %d/%s; want 200", loginResponse.Code, loginResponse.Body)
	}
	var staffLogin struct {
		Data struct {
			Token  string `json:"token"`
			UserID string `json:"user_id"`
			Role   string `json:"role"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&staffLogin); err != nil {
		t.Fatalf("decode staff login response: %v", err)
	}
	if staffLogin.Data.Token == "" || staffLogin.Data.Role != "owner" {
		t.Fatalf("staff login payload = %+v; want a token and owner role", staffLogin.Data)
	}
	staffAuthorization := "Bearer " + staffLogin.Data.Token
	protectedOwner := performAPIRequest(mux, http.MethodDelete, "/v1/admin/staff/"+staffLogin.Data.UserID, staffAuthorization, "")
	if protectedOwner.Code != http.StatusConflict {
		t.Fatalf("remove owner status/body = %d/%s; want 409", protectedOwner.Code, protectedOwner.Body)
	}
	meResponse := performAPIRequest(mux, http.MethodGet, "/v1/admin/me", staffAuthorization, "")
	if meResponse.Code != http.StatusOK || !strings.Contains(meResponse.Body.String(), `"role":"owner"`) {
		t.Fatalf("admin identity status/body = %d/%s; want owner identity", meResponse.Code, meResponse.Body)
	}
	orderList := performAPIRequest(mux, http.MethodGet, "/v1/admin/orders?limit=10", staffAuthorization, "")
	if orderList.Code != http.StatusOK || !strings.Contains(orderList.Body.String(), createdOrder.Data.ID) {
		t.Fatalf("admin order list status/body = %d/%s; want checkout order", orderList.Code, orderList.Body)
	}
	invalidOrderPage := performAPIRequest(mux, http.MethodGet, "/v1/admin/orders?limit=0", staffAuthorization, "")
	if invalidOrderPage.Code != http.StatusBadRequest {
		t.Fatalf("invalid order page status/body = %d/%s; want 400", invalidOrderPage.Code, invalidOrderPage.Body)
	}
	orderDetail := performAPIRequest(mux, http.MethodGet, "/v1/admin/orders/"+createdOrder.Data.ID, staffAuthorization, "")
	if orderDetail.Code != http.StatusOK || !strings.Contains(orderDetail.Body.String(), `"email":"checkout@example.com"`) ||
		!strings.Contains(orderDetail.Body.String(), `"sku":"CABAS-LIN-NAT"`) ||
		!strings.Contains(orderDetail.Body.String(), `"shipping_carrier_name":"Colissimo"`) ||
		!strings.Contains(orderDetail.Body.String(), `"shipping_service_name":"Domicile"`) ||
		!strings.Contains(orderDetail.Body.String(), `"shipping_rate_version":"`+homeQuote.RateVersion+`"`) ||
		!strings.Contains(orderDetail.Body.String(), `"shipping_zone_name":"France et Union européenne"`) {
		t.Fatalf("admin order detail status/body = %d/%s; want customer, line and shipping snapshots", orderDetail.Code, orderDetail.Body)
	}
	shippingConfigResponse := performAPIRequest(mux, http.MethodGet, "/v1/admin/shipping", staffAuthorization, "")
	if shippingConfigResponse.Code != http.StatusOK ||
		!strings.Contains(shippingConfigResponse.Body.String(), `"code":"colissimo"`) ||
		!strings.Contains(shippingConfigResponse.Body.String(), `"amount":"6.9000"`) {
		t.Fatalf("admin shipping config status/body = %d/%s", shippingConfigResponse.Code, shippingConfigResponse.Body)
	}
	replaceShipping := performAPIRequest(mux, http.MethodPut, "/v1/admin/shipping", staffAuthorization,
		`{"carriers":[{"code":"colissimo","name":"Colissimo","active":true}],"services":[{"carrier_code":"colissimo","code":"home","name":"Domicile","active":true},{"carrier_code":"colissimo","code":"express","name":"Express","active":true},{"carrier_code":"colissimo","code":"legacy","name":"Offre historique","active":true}],"zones":[{"code":"france-ue","name":"France et Union européenne","country_codes":["FR"],"active":true}],"rates":[{"carrier_code":"colissimo","service_code":"home","zone_code":"france-ue","amount":"6.90","currency":"EUR","active":true},{"carrier_code":"colissimo","service_code":"express","zone_code":"france-ue","amount":"12.90","currency":"EUR","active":true},{"carrier_code":"colissimo","service_code":"legacy","zone_code":"france-ue","amount":"6.90","currency":"EUR","active":true}]}`)
	if replaceShipping.Code != http.StatusOK || !strings.Contains(replaceShipping.Body.String(), `"service_code":"home"`) ||
		!strings.Contains(replaceShipping.Body.String(), `"service_code":"express"`) ||
		!strings.Contains(replaceShipping.Body.String(), `"rate_version":"`) {
		t.Fatalf("admin shipping update status/body = %d/%s; want saved tariff", replaceShipping.Code, replaceShipping.Body)
	}
	historyPath := "/v1/admin/shipping/versions?carrier_code=colissimo&service_code=home&zone_code=france-ue"
	shippingHistory := performAPIRequest(mux, http.MethodGet, historyPath, staffAuthorization, "")
	if shippingHistory.Code != http.StatusOK || !strings.Contains(shippingHistory.Body.String(), homeQuote.RateVersion) {
		t.Fatalf("admin shipping history status/body = %d/%s; want original home version", shippingHistory.Code, shippingHistory.Body)
	}
	shippingHistoryTargets := performAPIRequest(mux, http.MethodGet,
		"/v1/admin/shipping/versions/targets", staffAuthorization, "")
	if shippingHistoryTargets.Code != http.StatusOK ||
		!strings.Contains(shippingHistoryTargets.Body.String(), `"service_code":"home"`) {
		t.Fatalf("admin shipping history targets status/body = %d/%s; want historical service target",
			shippingHistoryTargets.Code, shippingHistoryTargets.Body)
	}
	invalidShippingHistory := performAPIRequest(mux, http.MethodGet,
		"/v1/admin/shipping/versions?carrier_code=colissimo", staffAuthorization, "")
	if invalidShippingHistory.Code != http.StatusBadRequest {
		t.Fatalf("incomplete shipping history query status/body = %d/%s; want 400",
			invalidShippingHistory.Code, invalidShippingHistory.Body)
	}
	invalidShippingHistoryLimit := performAPIRequest(mux, http.MethodGet,
		historyPath+"&limit=101", staffAuthorization, "")
	if invalidShippingHistoryLimit.Code != http.StatusBadRequest {
		t.Fatalf("invalid shipping history limit status/body = %d/%s; want 400",
			invalidShippingHistoryLimit.Code, invalidShippingHistoryLimit.Body)
	}
	restorePath := "/v1/admin/shipping/versions/" + homeQuote.RateVersion + "/restore"
	restoreShipping := performAPIRequest(mux, http.MethodPost, restorePath, staffAuthorization, "")
	if restoreShipping.Code != http.StatusOK {
		t.Fatalf("admin shipping restore status/body = %d/%s; want 200", restoreShipping.Code, restoreShipping.Body)
	}
	var restoredShipping struct {
		Data struct {
			RateVersion string `json:"rate_version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(restoreShipping.Body).Decode(&restoredShipping); err != nil ||
		restoredShipping.Data.RateVersion == "" || restoredShipping.Data.RateVersion == homeQuote.RateVersion {
		t.Fatalf("admin shipping restore response = %+v, error = %v; want a new rate version", restoredShipping, err)
	}
	missingShippingVersion := performAPIRequest(mux, http.MethodPost,
		"/v1/admin/shipping/versions/00000000-0000-4000-8000-000000000000/restore", staffAuthorization, "")
	if missingShippingVersion.Code != http.StatusNotFound {
		t.Fatalf("missing shipping version status/body = %d/%s; want 404",
			missingShippingVersion.Code, missingShippingVersion.Body)
	}
	restoredShippingConfig := performAPIRequest(mux, http.MethodGet, "/v1/admin/shipping", staffAuthorization, "")
	if restoredShippingConfig.Code != http.StatusOK ||
		!strings.Contains(restoredShippingConfig.Body.String(), `"rate_version":"`+restoredShipping.Data.RateVersion+`"`) {
		t.Fatalf("shipping config after restore status/body = %d/%s; want restored revision",
			restoredShippingConfig.Code, restoredShippingConfig.Body)
	}
	orderAfterShippingRestore := performAPIRequest(mux, http.MethodGet,
		"/v1/admin/orders/"+createdOrder.Data.ID, staffAuthorization, "")
	if orderAfterShippingRestore.Code != http.StatusOK ||
		!strings.Contains(orderAfterShippingRestore.Body.String(), `"shipping_rate_version":"`+homeQuote.RateVersion+`"`) {
		t.Fatalf("order after shipping restore status/body = %d/%s; want original order snapshot",
			orderAfterShippingRestore.Code, orderAfterShippingRestore.Body)
	}

	adminCategory := performAPIRequest(mux, http.MethodPost, "/v1/admin/categories", staffAuthorization,
		`{"slug":"integration-accessoires","translations":[{"locale":"fr-FR","name":"Accessoires"}]}`)
	if adminCategory.Code != http.StatusCreated {
		t.Fatalf("admin category create status/body = %d/%s; want 201", adminCategory.Code, adminCategory.Body)
	}
	createStaff := performAPIRequest(mux, http.MethodPost, "/v1/admin/staff", staffAuthorization,
		`{"email":"integration-editor@example.com","name":"Integration Editor","role":"editor","password":"initial-editor-password"}`)
	if createStaff.Code != http.StatusCreated || !strings.Contains(createStaff.Body.String(), `"role":"editor"`) {
		t.Fatalf("staff create status/body = %d/%s; want editor member", createStaff.Code, createStaff.Body)
	}
	var createdStaff struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createStaff.Body).Decode(&createdStaff); err != nil {
		t.Fatalf("decode created staff member: %v", err)
	}
	if createdStaff.Data.ID == "" {
		t.Fatal("created staff member response has no ID")
	}
	editorLogin := performAPIRequest(mux, http.MethodPost, "/v1/admin/login", "",
		`{"email":"integration-editor@example.com","password":"initial-editor-password"}`)
	if editorLogin.Code != http.StatusOK {
		t.Fatalf("editor login status/body = %d/%s; want 200", editorLogin.Code, editorLogin.Body)
	}
	var editorSession struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(editorLogin.Body).Decode(&editorSession); err != nil {
		t.Fatalf("decode editor login: %v", err)
	}
	editorStaffAccess := performAPIRequest(mux, http.MethodGet, "/v1/admin/staff", "Bearer "+editorSession.Data.Token, "")
	if editorStaffAccess.Code != http.StatusForbidden {
		t.Fatalf("editor staff-list status/body = %d/%s; want 403", editorStaffAccess.Code, editorStaffAccess.Body)
	}
	editorOrderAccess := performAPIRequest(mux, http.MethodGet, "/v1/admin/orders", "Bearer "+editorSession.Data.Token, "")
	if editorOrderAccess.Code != http.StatusForbidden {
		t.Fatalf("editor order-list status/body = %d/%s; want 403", editorOrderAccess.Code, editorOrderAccess.Body)
	}
	editorShippingWrite := performAPIRequest(mux, http.MethodPut, "/v1/admin/shipping", "Bearer "+editorSession.Data.Token, `{}`)
	if editorShippingWrite.Code != http.StatusForbidden {
		t.Fatalf("editor shipping-write status/body = %d/%s; want 403", editorShippingWrite.Code, editorShippingWrite.Body)
	}
	editorShippingHistory := performAPIRequest(mux, http.MethodGet, historyPath, "Bearer "+editorSession.Data.Token, "")
	if editorShippingHistory.Code != http.StatusForbidden {
		t.Fatalf("editor shipping-history status/body = %d/%s; want 403", editorShippingHistory.Code, editorShippingHistory.Body)
	}
	editorShippingRestore := performAPIRequest(mux, http.MethodPost, restorePath, "Bearer "+editorSession.Data.Token, "")
	if editorShippingRestore.Code != http.StatusForbidden {
		t.Fatalf("editor shipping-restore status/body = %d/%s; want 403", editorShippingRestore.Code, editorShippingRestore.Body)
	}
	staffList := performAPIRequest(mux, http.MethodGet, "/v1/admin/staff", staffAuthorization, "")
	if staffList.Code != http.StatusOK || !strings.Contains(staffList.Body.String(), "integration-editor@example.com") {
		t.Fatalf("staff list status/body = %d/%s; want new editor", staffList.Code, staffList.Body)
	}
	updateStaff := performAPIRequest(mux, http.MethodPut, "/v1/admin/staff/"+createdStaff.Data.ID, staffAuthorization,
		`{"email":"integration-editor@example.com","name":"Integration Support","role":"support","password":"replacement-editor-password"}`)
	if updateStaff.Code != http.StatusOK || !strings.Contains(updateStaff.Body.String(), `"role":"support"`) {
		t.Fatalf("staff update status/body = %d/%s; want support role", updateStaff.Code, updateStaff.Body)
	}
	revokedAfterReset := performAPIRequest(mux, http.MethodGet, "/v1/admin/me", "Bearer "+editorSession.Data.Token, "")
	if revokedAfterReset.Code != http.StatusUnauthorized {
		t.Fatalf("editor session after password reset status/body = %d/%s; want 401", revokedAfterReset.Code, revokedAfterReset.Body)
	}
	supportLogin := performAPIRequest(mux, http.MethodPost, "/v1/admin/login", "",
		`{"email":"integration-editor@example.com","password":"replacement-editor-password"}`)
	if supportLogin.Code != http.StatusOK || !strings.Contains(supportLogin.Body.String(), `"role":"support"`) {
		t.Fatalf("support login after reset status/body = %d/%s; want support session", supportLogin.Code, supportLogin.Body)
	}
	var supportSession struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(supportLogin.Body).Decode(&supportSession); err != nil {
		t.Fatalf("decode support login: %v", err)
	}
	supportOrders := performAPIRequest(mux, http.MethodGet, "/v1/admin/orders", "Bearer "+supportSession.Data.Token, "")
	if supportOrders.Code != http.StatusOK {
		t.Fatalf("support order list status/body = %d/%s; want 200", supportOrders.Code, supportOrders.Body)
	}
	supportCancel := performAPIRequest(mux, http.MethodPatch, "/v1/admin/orders/"+createdOrder.Data.ID+"/status",
		"Bearer "+supportSession.Data.Token, `{"status":"cancelled"}`)
	if supportCancel.Code != http.StatusForbidden {
		t.Fatalf("support cancellation status/body = %d/%s; want 403", supportCancel.Code, supportCancel.Body)
	}
	supportManualPayment := performAPIRequest(mux, http.MethodPost, "/v1/admin/orders/"+createdOrder.Data.ID+"/payments/manual",
		"Bearer "+supportSession.Data.Token, `{}`)
	if supportManualPayment.Code != http.StatusForbidden {
		t.Fatalf("support manual payment status/body = %d/%s; want 403", supportManualPayment.Code, supportManualPayment.Body)
	}
	supportFulfillment := performAPIRequest(mux, http.MethodPost, "/v1/admin/orders/"+createdOrder.Data.ID+"/fulfill",
		"Bearer "+supportSession.Data.Token, `{}`)
	if supportFulfillment.Code != http.StatusForbidden {
		t.Fatalf("support fulfillment status/body = %d/%s; want 403", supportFulfillment.Code, supportFulfillment.Body)
	}
	supportShippingWrite := performAPIRequest(mux, http.MethodPut, "/v1/admin/shipping", "Bearer "+supportSession.Data.Token, `{}`)
	if supportShippingWrite.Code != http.StatusForbidden {
		t.Fatalf("support shipping-write status/body = %d/%s; want 403", supportShippingWrite.Code, supportShippingWrite.Body)
	}
	manualFlowToken := sha256.Sum256([]byte("admin manual payment flow"))
	if err := db.CreateCart(context.Background(), seed.ShopID, manualFlowToken[:]); err != nil {
		t.Fatalf("create admin manual-payment cart: %v", err)
	}
	defer func() {
		_, _ = db.SQL.ExecContext(context.Background(), "DELETE FROM carts WHERE access_token_hash = $1", manualFlowToken[:])
	}()
	if _, err := db.AddCartLine(context.Background(), seed.ShopID, manualFlowToken[:], seed.VariantID, 1); err != nil {
		t.Fatalf("add admin manual-payment cart line: %v", err)
	}
	manualFlowOrder, err := db.CheckoutCart(context.Background(), seed.ShopID, manualFlowToken[:], store.CheckoutDetails{
		Email:               "admin-manual-flow@example.com",
		Name:                "Admin manual flow",
		ShippingCarrierCode: "colissimo",
		ShippingAddress: store.ShippingAddress{
			Line1: "1 rue du Test", City: "Paris", PostalCode: "75001", Country: "FR",
		},
	})
	if err != nil {
		t.Fatalf("create order for admin manual-payment flow: %v", err)
	}
	manualPaymentResponse := performAPIRequest(mux, http.MethodPost,
		"/v1/admin/orders/"+manualFlowOrder.ID+"/payments/manual", staffAuthorization,
		`{"provider_reference":"BANK-API-1"}`)
	if manualPaymentResponse.Code != http.StatusNoContent {
		t.Fatalf("admin manual payment status/body = %d/%s; want 204", manualPaymentResponse.Code, manualPaymentResponse.Body)
	}
	fulfillmentResponse := performAPIRequest(mux, http.MethodPost,
		"/v1/admin/orders/"+manualFlowOrder.ID+"/fulfill", staffAuthorization,
		`{"tracking":"TRACK-API-1"}`)
	if fulfillmentResponse.Code != http.StatusNoContent {
		t.Fatalf("admin fulfillment status/body = %d/%s; want 204", fulfillmentResponse.Code, fulfillmentResponse.Body)
	}
	fulfilledDetail := performAPIRequest(mux, http.MethodGet,
		"/v1/admin/orders/"+manualFlowOrder.ID, staffAuthorization, "")
	if fulfilledDetail.Code != http.StatusOK ||
		!strings.Contains(fulfilledDetail.Body.String(), `"status":"fulfilled"`) ||
		!strings.Contains(fulfilledDetail.Body.String(), `"shipment_status":"shipped"`) ||
		!strings.Contains(fulfilledDetail.Body.String(), `"tracking":"TRACK-API-1"`) {
		t.Fatalf("fulfilled admin order status/body = %d/%s", fulfilledDetail.Code, fulfilledDetail.Body)
	}
	cancelOrder := performAPIRequest(mux, http.MethodPatch, "/v1/admin/orders/"+createdOrder.Data.ID+"/status",
		staffAuthorization, `{"status":"cancelled"}`)
	if cancelOrder.Code != http.StatusNoContent {
		t.Fatalf("admin cancellation status/body = %d/%s; want 204", cancelOrder.Code, cancelOrder.Body)
	}
	cancelledDetail := performAPIRequest(mux, http.MethodGet, "/v1/admin/orders/"+createdOrder.Data.ID, staffAuthorization, "")
	if cancelledDetail.Code != http.StatusOK || !strings.Contains(cancelledDetail.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("cancelled order detail status/body = %d/%s; want cancelled status", cancelledDetail.Code, cancelledDetail.Body)
	}
	deleteStaff := performAPIRequest(mux, http.MethodDelete, "/v1/admin/staff/"+createdStaff.Data.ID, staffAuthorization, "")
	if deleteStaff.Code != http.StatusNoContent {
		t.Fatalf("staff remove status/body = %d/%s; want 204", deleteStaff.Code, deleteStaff.Body)
	}
	revokedAfterRemoval := performAPIRequest(mux, http.MethodGet, "/v1/admin/me", "Bearer "+supportSession.Data.Token, "")
	if revokedAfterRemoval.Code != http.StatusUnauthorized {
		t.Fatalf("support session after removal status/body = %d/%s; want 401", revokedAfterRemoval.Code, revokedAfterRemoval.Body)
	}

	var createdCategory struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(adminCategory.Body).Decode(&createdCategory); err != nil {
		t.Fatalf("decode admin category: %v", err)
	}
	if createdCategory.Data.ID == "" {
		t.Fatal("admin category response has no ID")
	}

	productBody := `{"slug":"integration-article","status":"active","translations":[{"locale":"fr-FR","name":"Article d’intégration"}],"category_ids":["` +
		createdCategory.Data.ID + `"],"variants":[{"sku":"INTEGRATION-ARTICLE-1","options":{"taille":"unique"},"weight_grams":250,"prices":[{"currency":"EUR","amount":"12.5000"}],"inventory":{"quantity":8,"track":true}}]}`
	adminProduct := performAPIRequest(mux, http.MethodPost, "/v1/admin/products", staffAuthorization, productBody)
	if adminProduct.Code != http.StatusCreated {
		t.Fatalf("admin product create status/body = %d/%s; want 201", adminProduct.Code, adminProduct.Body)
	}
	var createdProduct struct {
		Data struct {
			ID          string   `json:"id"`
			CategoryIDs []string `json:"category_ids"`
			Variants    []struct {
				ID          string `json:"id"`
				WeightGrams *int64 `json:"weight_grams"`
				Prices      []struct {
					Amount string `json:"amount"`
				} `json:"prices"`
				Inventory struct {
					Quantity int `json:"quantity"`
				} `json:"inventory"`
			} `json:"variants"`
		} `json:"data"`
	}
	if err := json.NewDecoder(adminProduct.Body).Decode(&createdProduct); err != nil {
		t.Fatalf("decode admin product: %v", err)
	}
	if createdProduct.Data.ID == "" || len(createdProduct.Data.CategoryIDs) != 1 ||
		createdProduct.Data.CategoryIDs[0] != createdCategory.Data.ID || len(createdProduct.Data.Variants) != 1 ||
		createdProduct.Data.Variants[0].Prices[0].Amount != "12.5000" ||
		createdProduct.Data.Variants[0].WeightGrams == nil ||
		*createdProduct.Data.Variants[0].WeightGrams != 250 ||
		createdProduct.Data.Variants[0].Inventory.Quantity != 8 {
		t.Fatalf("created admin product payload = %+v", createdProduct.Data)
	}
	updatedProductBody := `{"slug":"integration-article","status":"active","translations":[{"locale":"fr-FR","name":"Article mis à jour"}],"category_ids":["` +
		createdCategory.Data.ID + `"],"variants":[{"sku":"INTEGRATION-ARTICLE-1","options":{"taille":"unique"},"prices":[{"currency":"EUR","amount":"15.0000"}],"inventory":{"quantity":6,"track":true}}]}`
	updatedProduct := performAPIRequest(mux, http.MethodPut, "/v1/admin/products/"+createdProduct.Data.ID, staffAuthorization, updatedProductBody)
	if updatedProduct.Code != http.StatusOK ||
		!strings.Contains(updatedProduct.Body.String(), `"amount":"15.0000"`) ||
		!strings.Contains(updatedProduct.Body.String(), `"quantity":6`) {
		t.Fatalf("admin product update status/body = %d/%s; want saved price and inventory", updatedProduct.Code, updatedProduct.Body)
	}
	adminProductList := performAPIRequest(mux, http.MethodGet, "/v1/admin/products", staffAuthorization, "")
	if adminProductList.Code != http.StatusOK || !strings.Contains(adminProductList.Body.String(), createdProduct.Data.ID) {
		t.Fatalf("admin product list status/body = %d/%s; want created product", adminProductList.Code, adminProductList.Body)
	}
	archiveProduct := performAPIRequest(mux, http.MethodDelete, "/v1/admin/products/"+createdProduct.Data.ID, staffAuthorization, "")
	if archiveProduct.Code != http.StatusNoContent {
		t.Fatalf("admin product archive status/body = %d/%s; want 204", archiveProduct.Code, archiveProduct.Body)
	}
	deleteCategory := performAPIRequest(mux, http.MethodDelete, "/v1/admin/categories/"+createdCategory.Data.ID, staffAuthorization, "")
	if deleteCategory.Code != http.StatusNoContent {
		t.Fatalf("admin category delete status/body = %d/%s; want 204", deleteCategory.Code, deleteCategory.Body)
	}
	logoutResponse := performAPIRequest(mux, http.MethodPost, "/v1/admin/logout", staffAuthorization, "")
	if logoutResponse.Code != http.StatusNoContent {
		t.Fatalf("admin logout status/body = %d/%s; want 204", logoutResponse.Code, logoutResponse.Body)
	}
	rejectedAfterLogout := performAPIRequest(mux, http.MethodGet, "/v1/admin/me", staffAuthorization, "")
	if rejectedAfterLogout.Code != http.StatusUnauthorized {
		t.Fatalf("admin request after logout status/body = %d/%s; want 401", rejectedAfterLogout.Code, rejectedAfterLogout.Body)
	}
}

func performAPIRequest(mux *http.ServeMux, method, path, authorization, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	return response
}
