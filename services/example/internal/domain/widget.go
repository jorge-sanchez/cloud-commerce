// Package domain holds the Widget aggregate: entity, value objects,
// repository interface, and domain events. Business rules live here —
// services orchestrate, repositories persist.
package domain

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// WidgetStatus is the lifecycle state of a Widget.
type WidgetStatus string

const (
	WidgetStatusDraft     WidgetStatus = "draft"
	WidgetStatusPublished WidgetStatus = "published"
	WidgetStatusArchived  WidgetStatus = "archived"
)

// Domain sentinel errors for entity-level failures.
var (
	ErrNotPublishable = errors.New("widget cannot be published in its current status")
	ErrNotArchivable  = errors.New("widget cannot be archived in its current status")
)

// Widget is the aggregate root of this example service.
type Widget struct {
	ID        string
	TenantID  string
	Name      string
	Status    WidgetStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewWidget constructs a draft Widget. ID and timestamps are assigned by the
// repository on save.
func NewWidget(tenantID, name string) *Widget {
	return &Widget{
		TenantID: tenantID,
		Name:     name,
		Status:   WidgetStatusDraft,
	}
}

// CanPublish reports whether the widget may transition to published.
func (w *Widget) CanPublish() bool {
	return w.Status == WidgetStatusDraft
}

// Publish transitions the widget to published. The entity decides its own
// transitions — callers must not check or set Status directly.
func (w *Widget) Publish() error {
	if !w.CanPublish() {
		return fmt.Errorf("%w: status is %q", ErrNotPublishable, w.Status)
	}
	w.Status = WidgetStatusPublished
	return nil
}

// CanArchive reports whether the widget may transition to archived.
func (w *Widget) CanArchive() bool {
	return w.Status == WidgetStatusPublished
}

// Archive transitions the widget to archived.
func (w *Widget) Archive() error {
	if !w.CanArchive() {
		return fmt.Errorf("%w: status is %q", ErrNotArchivable, w.Status)
	}
	w.Status = WidgetStatusArchived
	return nil
}

// WidgetPublishedEventType is the envelope type for WidgetPublishedEvent.
const WidgetPublishedEventType = "widget.published"

// WidgetPublishedEvent is emitted after a widget is published. Build event
// payloads from the persisted domain object, not from the raw request.
type WidgetPublishedEvent struct {
	WidgetID    string    `json:"widget_id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
}

// NewWidgetPublishedEvent builds the event from the persisted widget.
func NewWidgetPublishedEvent(w *Widget, at time.Time) WidgetPublishedEvent {
	return WidgetPublishedEvent{
		WidgetID:    w.ID,
		TenantID:    w.TenantID,
		Name:        w.Name,
		PublishedAt: at,
	}
}

// WidgetRepository is the persistence port for the Widget aggregate.
// Method names express intent, not generic CRUD.
type WidgetRepository interface {
	// SaveNew persists a freshly constructed widget and returns the stored
	// row (with ID and timestamps assigned).
	SaveNew(ctx context.Context, tenantID string, w *Widget) (*Widget, error)
	// GetByID returns the widget or apperrors.ErrNotFound.
	GetByID(ctx context.Context, tenantID, id string) (*Widget, error)
	// PublishIfPublishable loads the widget, lets the entity decide the
	// transition, and persists the result. Returns apperrors.ErrConflict
	// when the entity rejects the transition.
	PublishIfPublishable(ctx context.Context, tenantID, id string) (*Widget, error)
	// ListByTenant returns one page of widgets plus the total count.
	ListByTenant(ctx context.Context, tenantID string, page, pageSize int) ([]*Widget, int, error)
}
