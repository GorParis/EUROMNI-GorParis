package store

import (
	"context"
	"errors"
	"sort"
	"time"
)

type memoryOrder struct {
	shopID            string
	view              *OrderView
	email             string
	name              string
	address           ShippingAddress
	createdAt         time.Time
	variantQuantities map[string]int
	payment           *memoryPayment
	shipmentStatus    string
	tracking          string
	lifecycleEvents   []OrderLifecycleEvent
	shippingZoneCode  string
	shippingZoneName  string
}

type memoryPayment struct {
	attempt   PaymentAttempt
	status    string
	eventID   string
	reference string
}

var (
	_ PaymentLifecycleWriter = (*Memory)(nil)
	_ OrderAdminReader       = (*Memory)(nil)
)

func cloneOrderView(order *OrderView) *OrderView {
	copy := *order
	copy.Items = append([]OrderLineView(nil), order.Items...)
	for i := range copy.Items {
		copy.Items[i].Personalizations = append([]PersonalizationSelection(nil), order.Items[i].Personalizations...)
		copy.Items[i].SupplierSnapshot = append([]SupplierSnapshotItem(nil), order.Items[i].SupplierSnapshot...)
	}
	copy.ShippingWeightGrams = cloneInt64(order.ShippingWeightGrams)
	copy.ShippingPricingSubtotal = cloneString(order.ShippingPricingSubtotal)
	copy.ShippingRateRule = append([]byte(nil), order.ShippingRateRule...)
	copy.ShippingPolicySnapshot = cloneShippingPolicySnapshot(order.ShippingPolicySnapshot)
	return &copy
}

func (m *Memory) CreateDevelopmentPaymentAttempt(ctx context.Context, shopID, orderID string) (*PaymentAttempt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return nil, ErrNotFound
	}
	if order.view.Status != "pending" || order.payment != nil {
		return nil, ErrOrderTransition
	}
	id, err := NewCatalogID()
	if err != nil {
		return nil, err
	}
	attempt := &PaymentAttempt{
		ID: id, OrderID: orderID, Provider: developmentPaymentProvider,
		Amount: order.view.GrandTotal, Currency: order.view.Currency, Status: "pending",
	}
	order.payment = &memoryPayment{attempt: *attempt, status: "pending"}
	return attempt, nil
}

func (m *Memory) ApplyDevelopmentPaymentResult(ctx context.Context, shopID string, result PaymentResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	paymentStatus, orderStatus, valid := DevelopmentPaymentStatuses(result.Outcome)
	if !valid || result.Provider != developmentPaymentProvider ||
		result.PaymentID == "" || result.OrderID == "" || result.EventID == "" ||
		result.Amount == "" || result.Currency == "" {
		return ErrInvalidPaymentResult
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[result.OrderID]
	if !ok || order.shopID != shopID || order.payment == nil ||
		order.payment.attempt.ID != result.PaymentID {
		return ErrNotFound
	}
	payment := order.payment
	if payment.attempt.Provider != result.Provider ||
		payment.attempt.Amount != result.Amount || payment.attempt.Currency != result.Currency ||
		order.view.GrandTotal != result.Amount || order.view.Currency != result.Currency {
		return ErrInvalidPaymentResult
	}
	if payment.status != "pending" {
		if payment.eventID == result.EventID && payment.status == paymentStatus &&
			payment.reference == result.Reference && order.view.Status == orderStatus {
			return nil
		}
		return ErrOrderTransition
	}
	if order.view.Status != "pending" {
		return ErrOrderTransition
	}
	for _, existingOrder := range m.orders {
		if existingOrder.payment != nil && existingOrder.payment.eventID == result.EventID {
			return ErrInvalidPaymentResult
		}
	}
	payment.status = paymentStatus
	payment.eventID = result.EventID
	payment.reference = result.Reference
	payment.attempt.Status = paymentStatus
	order.view.Status = orderStatus
	if orderStatus == "paid" {
		appendMemoryOrderLifecycleEvent(order, "paid", "payment", "")
	}
	if orderStatus == "cancelled" {
		m.restockMemoryOrder(order)
	}
	return nil
}

func (m *Memory) ListAdminOrders(ctx context.Context, shopID string, limit, offset int) ([]AdminOrderView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	orders := make([]*memoryOrder, 0, len(m.orders))
	for _, order := range m.orders {
		if order.shopID == shopID {
			orders = append(orders, order)
		}
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].createdAt.Equal(orders[j].createdAt) {
			return orders[i].view.ID > orders[j].view.ID
		}
		return orders[i].createdAt.After(orders[j].createdAt)
	})
	if offset >= len(orders) {
		return []AdminOrderView{}, nil
	}
	orders = orders[offset:]
	if limit >= 0 && limit < len(orders) {
		orders = orders[:limit]
	}
	result := make([]AdminOrderView, 0, len(orders))
	for _, order := range orders {
		result = append(result, m.adminOrderView(order))
	}
	return result, nil
}

func (m *Memory) GetAdminOrder(ctx context.Context, shopID, orderID string) (*AdminOrderView, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return nil, ErrNotFound
	}
	result := m.adminOrderView(order)
	return &result, nil
}

func (m *Memory) CancelAdminOrder(ctx context.Context, shopID, orderID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return ErrNotFound
	}
	if order.view.Status != "pending" || order.payment != nil {
		return ErrOrderTransition
	}
	order.view.Status = "cancelled"
	m.restockMemoryOrder(order)
	return nil
}

func (m *Memory) ConfirmManualOrderPayment(ctx context.Context, shopID, orderID, providerReference string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return ErrNotFound
	}
	if order.view.Status != "pending" || order.payment != nil {
		return ErrOrderTransition
	}
	order.payment = &memoryPayment{
		attempt: PaymentAttempt{
			ID: orderID, OrderID: orderID, Provider: "manual",
			Amount: order.view.GrandTotal, Currency: order.view.Currency, Status: "captured",
		},
		status: "captured", reference: providerReference,
	}
	order.view.Status = "paid"
	appendMemoryOrderLifecycleEvent(order, "paid", "payment", "")
	return nil
}

func (m *Memory) AdvanceAdminOrderLifecycle(ctx context.Context, shopID, orderID, eventType, tracking, source string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validOrderLifecycleEvent(eventType) || !validOrderLifecycleSource(source) {
		return ErrOrderTransition
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	order, ok := m.orders[orderID]
	if !ok || order.shopID != shopID {
		return ErrNotFound
	}
	for _, event := range order.lifecycleEvents {
		if event.EventType == eventType {
			return nil
		}
	}
	switch eventType {
	case "processing":
		if order.view.Status != "paid" {
			return ErrOrderTransition
		}
		order.view.Status = "fulfilling"
	case "prepared":
		if order.view.Status != "fulfilling" || order.view.ShippingCarrierCode == "" {
			return ErrOrderTransition
		}
		if order.shipmentStatus != "" && order.shipmentStatus != "pending" {
			return ErrOrderTransition
		}
		order.shipmentStatus = "ready"
	case "shipped":
		if order.view.Status != "fulfilling" || order.shipmentStatus != "ready" {
			return ErrOrderTransition
		}
		order.view.Status = "fulfilled"
		order.shipmentStatus = "shipped"
		order.tracking = tracking
	case "delivered":
		if order.view.Status != "fulfilled" || order.shipmentStatus != "shipped" {
			return ErrOrderTransition
		}
		order.shipmentStatus = "delivered"
	default:
		return ErrOrderTransition
	}
	appendMemoryOrderLifecycleEvent(order, eventType, source, tracking)
	return nil
}

func (m *Memory) FulfillAdminOrder(ctx context.Context, shopID, orderID, tracking string) error {
	order, err := m.GetAdminOrder(ctx, shopID, orderID)
	if err != nil {
		return err
	}
	if order.Status != "paid" && order.Status != "fulfilling" {
		return ErrOrderTransition
	}
	if err := m.AdvanceAdminOrderLifecycle(ctx, shopID, orderID, "processing", "", "staff"); err != nil {
		if !errors.Is(err, ErrOrderTransition) {
			return err
		}
	}
	if err := m.AdvanceAdminOrderLifecycle(ctx, shopID, orderID, "prepared", "", "staff"); err != nil {
		if !errors.Is(err, ErrOrderTransition) {
			return err
		}
	}
	return m.AdvanceAdminOrderLifecycle(ctx, shopID, orderID, "shipped", tracking, "staff")
}

func appendMemoryOrderLifecycleEvent(order *memoryOrder, eventType, source, tracking string) {
	order.lifecycleEvents = append(order.lifecycleEvents, OrderLifecycleEvent{
		EventType: eventType, Source: source, Tracking: tracking, OccurredAt: time.Now().UTC(),
	})
}

func (m *Memory) restockMemoryOrder(order *memoryOrder) {
	for variantID, quantity := range order.variantQuantities {
		for _, product := range m.products {
			for i := range product.Variants {
				variant := &product.Variants[i]
				if variant.ID == variantID && variant.Inventory.Track {
					variant.Inventory.Quantity += quantity
				}
			}
		}
	}
	clear(order.variantQuantities)
}

func memoryAdminOrderView(order *memoryOrder) AdminOrderView {
	view := order.view
	items := make([]OrderLineView, len(view.Items))
	copy(items, view.Items)
	for i := range items {
		items[i].Personalizations = append([]PersonalizationSelection(nil), view.Items[i].Personalizations...)
	}
	return AdminOrderView{
		ID: view.ID, Number: view.Number, Status: view.Status, Currency: view.Currency,
		Subtotal: view.Subtotal, TaxTotal: view.TaxTotal, ShippingTotal: view.ShippingTotal,
		ShippingCarrierCode: view.ShippingCarrierCode, ShippingCarrierName: view.ShippingCarrierName,
		ShippingServiceCode: view.ShippingServiceCode, ShippingServiceName: view.ShippingServiceName,
		ShippingRateVersion: view.ShippingRateVersion, ShippingZoneCode: order.shippingZoneCode,
		ShippingZoneName: order.shippingZoneName, ShippingRateMode: view.ShippingRateMode,
		ShippingWeightGrams:     cloneInt64(view.ShippingWeightGrams),
		ShippingPricingSubtotal: cloneString(view.ShippingPricingSubtotal),
		ShippingRateRule:        append([]byte(nil), view.ShippingRateRule...),
		ShippingPolicySnapshot:  cloneShippingPolicySnapshot(view.ShippingPolicySnapshot),
		ShipmentStatus:          order.shipmentStatus, Tracking: order.tracking, GrandTotal: view.GrandTotal,
		Email: order.email, Name: order.name, ShippingAddress: order.address,
		CreatedAt: order.createdAt, Items: items,
		LifecycleEvents: append([]OrderLifecycleEvent(nil), order.lifecycleEvents...),
	}
}

func (m *Memory) adminOrderView(order *memoryOrder) AdminOrderView {
	view := memoryAdminOrderView(order)
	if syncState := m.supplierSyncs[order.view.ID+"/selfnamed"]; syncState != nil {
		status := syncState.status
		view.SupplierOrderSync = &status
	}
	return view
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneShippingPolicySnapshot(value *ShippingPricingPolicySnapshot) *ShippingPricingPolicySnapshot {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
