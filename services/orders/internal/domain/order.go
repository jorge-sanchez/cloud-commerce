// Package domain holds the Cart and Order aggregates, domain events, and
// the repository interface. Business rules live here — services
// orchestrate, repositories persist.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// OrderStatus is the lifecycle state of an order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusFulfilled OrderStatus = "fulfilled"
	OrderStatusCancelled OrderStatus = "cancelled"
	OrderStatusRefunded  OrderStatus = "refunded"
)

// Domain sentinel errors for entity-level failures.
var (
	ErrBadQty         = errors.New("item quantity must be positive")
	ErrEmptyCart      = errors.New("an empty cart cannot be checked out")
	ErrInvalidEmail   = errors.New("email address is not valid")
	ErrNotPayable     = errors.New("order cannot be marked paid in its current status")
	ErrNotFulfillable = errors.New("order cannot be fulfilled in its current status")
	ErrNotCancellable = errors.New("order cannot be cancelled in its current status")
	ErrNotRefundable  = errors.New("order cannot be refunded in its current status")
	ErrBadAddress     = errors.New("shipping address is incomplete")
	ErrNoShipping     = errors.New("a shipping method is required at checkout")
)

// Item is a priced line: the snapshot taken when the buyer added it. Later
// catalog edits never change a cart or an order.
type Item struct {
	VariantID  string
	SKU        string
	Title      string
	PriceCents int64
	Qty        int64
}

// TaxSpec is the resolved jurisdiction rate applied to an order
// (RFC-002): rate in basis points, whether it covers shipping, and the
// store's pricing mode at order time.
type TaxSpec struct {
	Name              string
	RateBps           int
	AppliesToShipping bool
	Inclusive         bool
}

// halfUp rounds n*bps/10000 (or the inclusive extraction) half-up.
func halfUp(numerator, denominator int64) int64 {
	return (numerator + denominator/2) / denominator
}

// computeTax returns the tax amount for the given bases. Exclusive: tax
// is added on top. Inclusive: tax is the portion already inside the
// price — the total does not change.
func computeTax(itemsCents, shippingCents int64, t TaxSpec) int64 {
	if t.RateBps <= 0 {
		return 0
	}
	taxable := itemsCents
	if t.AppliesToShipping {
		taxable += shippingCents
	}
	if t.Inclusive {
		return halfUp(taxable*int64(t.RateBps), int64(10000+t.RateBps))
	}
	return halfUp(taxable*int64(t.RateBps), 10000)
}

// Address is the structured shipping snapshot (RFC-001): what was true
// at checkout, immutable afterwards.
type Address struct {
	Name    string
	Line1   string
	Line2   string
	City    string
	Region  string
	Postal  string
	Country string // ISO 3166-1 alpha-2
	Phone   string
}

// Validate enforces the minimal completeness rule — no verification
// service at launch (RFC-001).
func (a Address) Validate() error {
	if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Line1) == "" ||
		strings.TrimSpace(a.City) == "" || len(strings.TrimSpace(a.Country)) != 2 {
		return ErrBadAddress
	}
	return nil
}

// Cart is the buyer's aggregate. Its unguessable ID is the buyer's
// capability — buyers carry no tenant claim; the cart carries the tenant.
type Cart struct {
	ID           string
	TenantID     string
	Currency     string
	TaxInclusive bool // the store's pricing mode when the cart was created
	Items        []Item
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AddItem adds a snapshot line; adding the same variant again accumulates
// quantity (the snapshot from the first add wins — one price per line).
func (c *Cart) AddItem(item Item) error {
	if item.Qty <= 0 {
		return fmt.Errorf("%w: %d", ErrBadQty, item.Qty)
	}
	for i := range c.Items {
		if c.Items[i].VariantID == item.VariantID {
			c.Items[i].Qty += item.Qty
			return nil
		}
	}
	c.Items = append(c.Items, item)
	return nil
}

// RemoveItem drops a line; removing an absent variant is a no-op.
func (c *Cart) RemoveItem(variantID string) {
	for i := range c.Items {
		if c.Items[i].VariantID == variantID {
			c.Items = append(c.Items[:i], c.Items[i+1:]...)
			return
		}
	}
}

// TotalCents is the cart's current total.
func (c *Cart) TotalCents() int64 {
	var total int64
	for _, it := range c.Items {
		total += it.PriceCents * it.Qty
	}
	return total
}

// Order is the aggregate created at checkout. It owns its items; they are
// persisted with it atomically.
type Order struct {
	ID               string
	Number           int64
	TenantID         string
	Email            string
	Currency         string
	Items            []Item
	TotalCents       int64
	Status           OrderStatus
	ShippingAddress  Address
	ShippingMethod   string
	ShippingCents    int64
	TaxCents         int64
	TaxName          string
	TaxRateBps       int
	TaxInclusive     bool
	LocationID       string // POS: registering location; empty online
	PaymentReference string
	TrackingNumber   string
	Carrier          string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NewPOSSale builds an in-person sale: an order born paid (ADR-010).
// Buyer email is optional — cash buyers often give none.
func NewPOSSale(tenantID, currency, email, locationID string, items []Item, tax TaxSpec) (*Order, error) {
	if len(items) == 0 {
		return nil, ErrEmptyCart
	}
	var total int64
	for _, it := range items {
		if it.Qty <= 0 {
			return nil, fmt.Errorf("%w: %d", ErrBadQty, it.Qty)
		}
		total += it.PriceCents * it.Qty
	}
	return &Order{
		TenantID:       tenantID,
		Email:          strings.ToLower(strings.TrimSpace(email)),
		Currency:       currency,
		Items:          items,
		ShippingMethod: "in-store",
		LocationID:     locationID,
		TaxCents:       computeTax(total, 0, tax),
		TaxName:        tax.Name,
		TaxRateBps:     tax.RateBps,
		TaxInclusive:   tax.Inclusive,
		TotalCents:     totalWithTax(total, 0, tax),
		Status:         OrderStatusPaid,
	}, nil
}

// NewOrderFromCart converts a cart into a pending order. The entity
// decides: empty carts, invalid emails, incomplete addresses, and a
// missing shipping method are rejected. Total = items + shipping.
func NewOrderFromCart(cart *Cart, email string, addr Address, shippingMethod string, shippingCents int64, tax TaxSpec) (*Order, error) {
	if len(cart.Items) == 0 {
		return nil, ErrEmptyCart
	}
	if err := addr.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(shippingMethod) == "" || shippingCents < 0 {
		return nil, ErrNoShipping
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	at := strings.Index(normalized, "@")
	if at < 1 || at == len(normalized)-1 || !strings.Contains(normalized[at:], ".") {
		return nil, fmt.Errorf("%w: %q", ErrInvalidEmail, email)
	}
	items := make([]Item, len(cart.Items))
	copy(items, cart.Items)
	return &Order{
		TenantID:        cart.TenantID,
		Email:           normalized,
		Currency:        cart.Currency,
		Items:           items,
		ShippingAddress: addr,
		ShippingMethod:  strings.TrimSpace(shippingMethod),
		ShippingCents:   shippingCents,
		TaxCents:        computeTax(cart.TotalCents(), shippingCents, tax),
		TaxName:         tax.Name,
		TaxRateBps:      tax.RateBps,
		TaxInclusive:    tax.Inclusive,
		TotalCents:      totalWithTax(cart.TotalCents(), shippingCents, tax),
		Status:          OrderStatusPending,
	}, nil
}

// CanPay reports whether payment may be initialized for this order.
func (o *Order) CanPay() bool {
	return o.Status == OrderStatusPending
}

// totalWithTax: exclusive adds tax on top; inclusive leaves the total
// unchanged (the tax already lives inside the prices).
func totalWithTax(itemsCents, shippingCents int64, t TaxSpec) int64 {
	total := itemsCents + shippingCents
	if !t.Inclusive {
		total += computeTax(itemsCents, shippingCents, t)
	}
	return total
}

// MarkPaid transitions pending → paid. The entity decides its own
// transitions — callers must not check or set Status directly.
func (o *Order) MarkPaid() error {
	if o.Status != OrderStatusPending {
		return fmt.Errorf("%w: status is %q", ErrNotPayable, o.Status)
	}
	o.Status = OrderStatusPaid
	return nil
}

// Fulfill transitions paid → fulfilled, attaching optional tracking.
func (o *Order) Fulfill(trackingNumber, carrier string) error {
	if o.Status != OrderStatusPaid {
		return fmt.Errorf("%w: status is %q", ErrNotFulfillable, o.Status)
	}
	o.Status = OrderStatusFulfilled
	o.TrackingNumber = strings.TrimSpace(trackingNumber)
	o.Carrier = strings.TrimSpace(carrier)
	return nil
}

// Refund transitions paid or fulfilled → refunded (returns happen after
// shipping too).
func (o *Order) Refund() error {
	if o.Status != OrderStatusPaid && o.Status != OrderStatusFulfilled {
		return fmt.Errorf("%w: status is %q", ErrNotRefundable, o.Status)
	}
	o.Status = OrderStatusRefunded
	return nil
}

// Cancel transitions pending → cancelled.
func (o *Order) Cancel() error {
	if o.Status != OrderStatusPending {
		return fmt.Errorf("%w: status is %q", ErrNotCancellable, o.Status)
	}
	o.Status = OrderStatusCancelled
	return nil
}

// Event types for the order aggregate.
const (
	OrderPlacedEventType    = "orders.order_placed"
	OrderPaidEventType      = "orders.order_paid"
	OrderFulfilledEventType = "orders.order_fulfilled"
	OrderRefundedEventType  = "orders.order_refunded"
)

// OrderRefundedEvent is emitted at refund; inventory restores stock from it.
type OrderRefundedEvent struct {
	OrderID    string      `json:"order_id"`
	TenantID   string      `json:"tenant_id"`
	Items      []EventItem `json:"items"`
	RefundedAt time.Time   `json:"refunded_at"`
}

// NewOrderRefundedEvent builds the event from the persisted order.
func NewOrderRefundedEvent(o *Order, at time.Time) OrderRefundedEvent {
	return OrderRefundedEvent{OrderID: o.ID, TenantID: o.TenantID, Items: eventItems(o), RefundedAt: at}
}

// OrderFulfilledEvent is emitted at fulfillment; notifications will consume
// it (buyer email travels in the event so consumers need no order lookup).
type OrderFulfilledEvent struct {
	OrderID        string    `json:"order_id"`
	Number         int64     `json:"number"`
	TenantID       string    `json:"tenant_id"`
	Email          string    `json:"email"`
	TrackingNumber string    `json:"tracking_number"`
	Carrier        string    `json:"carrier"`
	FulfilledAt    time.Time `json:"fulfilled_at"`
}

// NewOrderFulfilledEvent builds the event from the persisted order.
func NewOrderFulfilledEvent(o *Order, at time.Time) OrderFulfilledEvent {
	return OrderFulfilledEvent{
		OrderID:        o.ID,
		Number:         o.Number,
		TenantID:       o.TenantID,
		Email:          o.Email,
		TrackingNumber: o.TrackingNumber,
		Carrier:        o.Carrier,
		FulfilledAt:    at,
	}
}

// EventItem is the wire shape of a line inside order events.
type EventItem struct {
	VariantID  string `json:"variant_id"`
	SKU        string `json:"sku"`
	PriceCents int64  `json:"price_cents"`
	Qty        int64  `json:"qty"`
}

// OrderPlacedEvent is emitted at checkout.
type OrderPlacedEvent struct {
	OrderID    string      `json:"order_id"`
	Number     int64       `json:"number"`
	TenantID   string      `json:"tenant_id"`
	TotalCents int64       `json:"total_cents"`
	Currency   string      `json:"currency"`
	Items      []EventItem `json:"items"`
	PlacedAt   time.Time   `json:"placed_at"`
}

// NewOrderPlacedEvent builds the event from the persisted order.
func NewOrderPlacedEvent(o *Order, at time.Time) OrderPlacedEvent {
	return OrderPlacedEvent{
		OrderID:    o.ID,
		Number:     o.Number,
		TenantID:   o.TenantID,
		TotalCents: o.TotalCents,
		Currency:   o.Currency,
		Items:      eventItems(o),
		PlacedAt:   at,
	}
}

// OrderPaidEvent is emitted when payment confirms; inventory consumes it to
// decrement stock (#18).
// Tax fields ride every money event (receipts render them).
type OrderPaidEvent struct {
	TaxCents     int64  `json:"tax_cents"`
	TaxName      string `json:"tax_name"`
	TaxRateBps   int    `json:"tax_rate_bps"`
	TaxInclusive bool   `json:"tax_inclusive"`
	// LocationID is set for POS sales: inventory deducts there instead
	// of the default location (RFC-001 acceptance resolution).
	LocationID string      `json:"location_id,omitempty"`
	OrderID    string      `json:"order_id"`
	Number     int64       `json:"number"`
	TenantID   string      `json:"tenant_id"`
	Email      string      `json:"email"`
	TotalCents int64       `json:"total_cents"`
	Currency   string      `json:"currency"`
	Items      []EventItem `json:"items"`
	PaidAt     time.Time   `json:"paid_at"`
}

// NewOrderPaidEvent builds the event from the persisted order.
func NewOrderPaidEvent(o *Order, at time.Time) OrderPaidEvent {
	return OrderPaidEvent{TaxCents: o.TaxCents, TaxName: o.TaxName, TaxRateBps: o.TaxRateBps, TaxInclusive: o.TaxInclusive, LocationID: o.LocationID, OrderID: o.ID, Number: o.Number, TenantID: o.TenantID, Email: o.Email, TotalCents: o.TotalCents, Currency: o.Currency, Items: eventItems(o), PaidAt: at}
}

func eventItems(o *Order) []EventItem {
	items := make([]EventItem, 0, len(o.Items))
	for _, it := range o.Items {
		items = append(items, EventItem{VariantID: it.VariantID, SKU: it.SKU, PriceCents: it.PriceCents, Qty: it.Qty})
	}
	return items
}

// DailySales is one day of revenue (paid + fulfilled orders).
type DailySales struct {
	Date         string
	RevenueCents int64
	Orders       int
}

// TopProduct is a best-selling variant by units.
type TopProduct struct {
	SKU          string
	Title        string
	Units        int64
	RevenueCents int64
}

// SalesSummary is the merchant analytics read.
type SalesSummary struct {
	Currency    string
	Days        []DailySales
	TopProducts []TopProduct
}

// OrderRepository is the persistence port for carts and orders.
type OrderRepository interface {
	// SaveNewCart persists an empty cart bound to a tenant and currency.
	SaveNewCart(ctx context.Context, cart *Cart) (*Cart, error)
	// GetCart returns the cart with items, or apperrors.ErrNotFound. Looked
	// up by unguessable ID only — the buyer's capability (see Cart).
	GetCart(ctx context.Context, cartID string) (*Cart, error)
	// ReplaceItems persists the cart's current lines atomically.
	ReplaceItems(ctx context.Context, cart *Cart) (*Cart, error)
	// PlaceOrderFromCart converts the cart into a pending order in one
	// transaction: order + items inserted, cart deleted, order_placed
	// event recorded. The entity decides (empty cart, bad email,
	// incomplete address, missing shipping).
	PlaceOrderFromCart(ctx context.Context, cartID, email string, addr Address, shippingMethod string, shippingCents int64, tax TaxSpec) (*Order, error)
	// MarkPaidIfPayable loads the order, lets the entity decide the
	// transition, persists it with the provider payment reference, and
	// records order_paid. Returns apperrors.ErrConflict when rejected.
	MarkPaidIfPayable(ctx context.Context, orderID, paymentReference string) (*Order, error)
	// RefundIfRefundable loads the order tenant-scoped, lets the entity
	// decide, persists it, and records order_refunded.
	RefundIfRefundable(ctx context.Context, tenantID, orderID string) (*Order, error)
	// FulfillIfFulfillable loads the order tenant-scoped, lets the entity
	// decide the transition, persists it with tracking, and records
	// order_fulfilled. Returns apperrors.ErrConflict when rejected.
	FulfillIfFulfillable(ctx context.Context, tenantID, orderID, trackingNumber, carrier string) (*Order, error)
	// GetByID returns the order with items, tenant-scoped.
	GetByID(ctx context.Context, tenantID, orderID string) (*Order, error)
	// GetPublicByID returns the order by unguessable ID alone — the
	// buyer's capability from checkout (see Cart).
	GetPublicByID(ctx context.Context, orderID string) (*Order, error)
	// SavePOSSale persists an in-person sale as an already-paid order in
	// one transaction (items + order_paid event), idempotent on the
	// client-generated sale ID: a replay returns the original order.
	SavePOSSale(ctx context.Context, tenantID, clientSaleID string, order *Order) (*Order, error)
	// ListByTenant returns one page of orders (with items), newest first.
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*Order, int, error)
	// GetSalesSummary aggregates revenue per day and top variants over the
	// last N days (paid and fulfilled orders count; refunds do not).
	GetSalesSummary(ctx context.Context, tenantID string, days int) (*SalesSummary, error)
}
