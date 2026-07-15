// Package service holds the application services: orchestration only, no
// business rules. Required dependencies are positional constructor arguments;
// optional dependencies are Option functions.
package service

import (
	"context"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/domain"
)

// WidgetService is the application-service port consumed by the handlers.
type WidgetService interface {
	Create(ctx context.Context, tenantID, name string) (*domain.Widget, error)
	Get(ctx context.Context, tenantID, id string) (*domain.Widget, error)
	Publish(ctx context.Context, tenantID, id string) (*domain.Widget, error)
	List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Widget, int, error)
}

type widgetService struct {
	repo domain.WidgetRepository // required
}

// Option configures optional dependencies on the widget service.
type Option func(*widgetService)

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
	// The WidgetPublished event is recorded by the repository in the same
	// transaction as the status change (transactional outbox, ADR-002); the
	// relay owns delivery. The service has nothing to publish here.
	return s.repo.PublishIfPublishable(ctx, tenantID, id)
}

func (s *widgetService) List(ctx context.Context, tenantID string, page, pageSize int) ([]*domain.Widget, int, error) {
	return s.repo.ListByTenant(ctx, tenantID, page, pageSize)
}
