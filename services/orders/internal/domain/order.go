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

// Cart is the buyer's aggregate. Its unguessable ID is the buyer's
// capability — buyers carry no tenant claim; the cart carries the tenant.
type Cart struct {
	ID        string
	TenantID  string
	Currency  string
	Items     []Item
	CreatedAt time.Time
	UpdatedAt time.Time
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
	PaymentReference string
	TrackingNumber   string
	Carrier          string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NewOrderFromCart converts a cart into a pending order. The entity
// decides: empty carts and invalid emails are rejected.
func NewOrderFromCart(cart *Cart, email string) (*Order, error) {
	if len(cart.Items) == 0 {
		return nil, ErrEmptyCart
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	at := strings.Index(normalized, "@")
	if at < 1 || at == len(normalized)-1 || !strings.Contains(normalized[at:], ".") {
		return nil, fmt.Errorf("%w: %q", ErrInvalidEmail, email)
	}
	items := make([]Item, len(cart.Items))
	copy(items, cart.Items)
	return &Order{
		TenantID:   cart.TenantID,
		Email:      normalized,
		Currency:   cart.Currency,
		Items:      items,
		TotalCents: cart.TotalCents(),
		Status:     OrderStatusPending,
	}, nil
}

// CanPay reports whether payment may be initialized for this order.
func (o *Order) CanPay() bool {
	return o.Status == OrderStatusPending
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
type OrderPaidEvent struct {
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
	return OrderPaidEvent{OrderID: o.ID, Number: o.Number, TenantID: o.TenantID, Email: o.Email, TotalCents: o.TotalCents, Currency: o.Currency, Items: eventItems(o), PaidAt: at}
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
	// event recorded. The entity decides (empty cart, bad email).
	PlaceOrderFromCart(ctx context.Context, cartID, email string) (*Order, error)
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
	// ListByTenant returns one page of orders (with items), newest first.
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*Order, int, error)
	// GetSalesSummary aggregates revenue per day and top variants over the
	// last N days (paid and fulfilled orders count; refunds do not).
	GetSalesSummary(ctx context.Context, tenantID string, days int) (*SalesSummary, error)
}
