// Package domain holds the inventory aggregates: locations and stock
// levels, domain events, and the repository interface. Business rules live
// here — services orchestrate, repositories persist.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Domain sentinel errors for entity-level failures.
var (
	ErrEmptyLocationName = errors.New("location name must not be empty")
	ErrInsufficientStock = errors.New("adjustment would drive stock below zero")
	ErrZeroAdjustment    = errors.New("adjustment delta must not be zero")
)

// DefaultLocationName names the location created automatically for every
// tenant the first time inventory sees them.
const DefaultLocationName = "Default"

// Location is a place stock is held (shop, warehouse, …).
type Location struct {
	ID        string
	TenantID  string
	Name      string
	IsDefault bool
	CreatedAt time.Time
}

// NewLocation constructs a non-default location. ID and timestamps are
// assigned by the repository on save.
func NewLocation(tenantID, name string) (*Location, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyLocationName
	}
	return &Location{TenantID: tenantID, Name: strings.TrimSpace(name)}, nil
}

// StockLevel is the on-hand quantity of one variant at one location.
// SKU is denormalized from the catalog event so inventory reads never need
// a cross-service call.
type StockLevel struct {
	ID         string
	TenantID   string
	LocationID string
	VariantID  string
	SKU        string
	OnHand     int64
	UpdatedAt  time.Time
}

// Adjust applies a delta. The entity forbids negative on-hand — reservations
// and oversells become their own concepts in Phase 2, not negative numbers.
func (s *StockLevel) Adjust(delta int64) error {
	if delta == 0 {
		return ErrZeroAdjustment
	}
	if s.OnHand+delta < 0 {
		return fmt.Errorf("%w: on hand %d, delta %d", ErrInsufficientStock, s.OnHand, delta)
	}
	s.OnHand += delta
	return nil
}

// DeductClamped removes qty but never goes below zero (paid orders must
// apply even when stock drifted — an oversell is reported, not refused).
// It returns the shortfall (0 when stock covered the deduction).
func (s *StockLevel) DeductClamped(qty int64) int64 {
	if qty >= s.OnHand {
		short := qty - s.OnHand
		s.OnHand = 0
		return short
	}
	s.OnHand -= qty
	return 0
}

// StockAdjustedEventType is the envelope type for StockAdjustedEvent.
const StockAdjustedEventType = "inventory.stock_adjusted"

// StockAdjustedEvent is emitted when on-hand stock changes.
type StockAdjustedEvent struct {
	VariantID  string    `json:"variant_id"`
	SKU        string    `json:"sku"`
	LocationID string    `json:"location_id"`
	Delta      int64     `json:"delta"`
	OnHand     int64     `json:"on_hand"`
	AdjustedAt time.Time `json:"adjusted_at"`
}

// NewStockAdjustedEvent builds the event from the persisted stock level.
func NewStockAdjustedEvent(s *StockLevel, delta int64, at time.Time) StockAdjustedEvent {
	return StockAdjustedEvent{
		VariantID:  s.VariantID,
		SKU:        s.SKU,
		LocationID: s.LocationID,
		Delta:      delta,
		OnHand:     s.OnHand,
		AdjustedAt: at,
	}
}

// StockInit seeds a zero-stock row for a variant (from catalog events).
type StockInit struct {
	VariantID string
	SKU       string
}

// StockDeduction is one order line to remove from stock.
type StockDeduction struct {
	VariantID string
	Qty       int64
}

// StockRepository is the persistence port for inventory.
type StockRepository interface {
	// EnsureDefaultLocation returns the tenant's default location, creating
	// it if this tenant has never been seen. Idempotent.
	EnsureDefaultLocation(ctx context.Context, tenantID string) (*Location, error)
	// SaveNewLocation persists a new non-default location.
	SaveNewLocation(ctx context.Context, tenantID string, l *Location) (*Location, error)
	// ListLocations returns the tenant's locations, default first.
	ListLocations(ctx context.Context, tenantID string) ([]*Location, error)
	// InitializeStock inserts zero-stock rows for the given variants at a
	// location. Rows that already exist are left untouched — the catalog
	// event that triggers this is delivered at-least-once (ADR-002).
	InitializeStock(ctx context.Context, tenantID, locationID string, items []StockInit) error
	// ListStockByTenant returns one page of stock levels plus the total.
	ListStockByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*StockLevel, int, error)
	// AdjustIfSufficient loads the stock level, lets the entity apply the
	// delta, and persists the result. Returns apperrors.ErrConflict when
	// the entity rejects the adjustment.
	AdjustIfSufficient(ctx context.Context, tenantID, locationID, variantID string, delta int64) (*StockLevel, error)
	// ApplyStockDeduction removes order quantities at the default location
	// in one transaction, deduped by event ID: replays are no-ops
	// (order_paid is delivered at-least-once). Missing rows are skipped;
	// insufficiency clamps at zero (the entity decides the clamp).
	ApplyStockDeduction(ctx context.Context, tenantID, eventID string, items []StockDeduction) error
}
