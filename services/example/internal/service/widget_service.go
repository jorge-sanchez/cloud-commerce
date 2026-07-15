// Package service holds the application services: orchestration only, no
// business rules. Required dependencies are positional constructor arguments;
// optional dependencies are Option functions.
package service

import (
	"context"
	"time"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/domain"
)

// EventPublisher is the outbound-events port. The service depends on the
// interface, not the concrete implementation.
type EventPublisher interface {
	PublishWidgetPublished(ctx context.Context, event domain.WidgetPublishedEvent) error
}

// WidgetService is the application-service port consumed by the handlers.
type WidgetService interface {
	Create(ctx context.Context, tenantID, name string) (*domain.Widget, error)
	Get(ctx context.Context, tenantID, id string) (*domain.Widget, error)
	Publish(ctx context.Context, tenantID, id string) (*domain.Widget, error)
	List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Widget, int, error)
}

type widgetService struct {
	repo   domain.WidgetRepository // required
	events EventPublisher          // may be nil
}

// Option configures optional dependencies on the widget service.
type Option func(*widgetService)

// WithEventPublisher wires an outbound event publisher. Without it, publish
// events are silently skipped.
func WithEventPublisher(ep EventPublisher) Option {
	return func(s *widgetService) { s.events = ep }
}

// NewWidgetService constructs the widget application service.
func NewWidgetService(repo domain.WidgetRepository, opts ...Option) WidgetService {
	s := &widgetService{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *widgetService) Create(ctx context.Context, tenantID, name string) (*domain.Widget, error) {
	if name == "" {
		return nil, apperrors.ErrValidation
	}
	return s.repo.SaveNew(ctx, tenantID, domain.NewWidget(tenantID, name))
}

func (s *widgetService) Get(ctx context.Context, tenantID, id string) (*domain.Widget, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *widgetService) Publish(ctx context.Context, tenantID, id string) (*domain.Widget, error) {
	published, err := s.repo.PublishIfPublishable(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}

	if s.events != nil {
		// Build the event from the persisted domain object, not the request.
		event := domain.WidgetPublishedEvent{
			WidgetID:    published.ID,
			TenantID:    published.TenantID,
			Name:        published.Name,
			PublishedAt: time.Now().UTC(),
		}
		// Publish failures are not surfaced once the row is persisted — the
		// state change is durable; a recovery process owns retries.
		_ = s.events.PublishWidgetPublished(ctx, event)
	}

	return published, nil
}

func (s *widgetService) List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Widget, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, page, pageSize)
}
