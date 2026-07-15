// Test Budget: 3 distinct behaviors × 2 = 6 max unit tests
// Actual: 5
//
// Behavior 1: Create — persists a draft widget; empty name is a validation error
// Behavior 2: Get — delegates to repo and passes errors through
// Behavior 3: Publish — delegates to repo and passes errors through (the
//
//	WidgetPublished event is recorded by the repository inside the publish
//	transaction — see the repository integration tests, ADR-002)
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/example/internal/domain"
)

// ---------------------------------------------------------------------------
// Hand-rolled fakes at the port boundaries — no gomock, no testify/mock.
// Call history is tracked in the fake struct for post-test assertions.
// ---------------------------------------------------------------------------

var _ domain.WidgetRepository = (*fakeWidgetRepo)(nil)

type fakeWidgetRepo struct {
	saved     []*domain.Widget
	getResult *domain.Widget
	getErr    error
	pubResult *domain.Widget
	pubErr    error
}

func (f *fakeWidgetRepo) SaveNew(_ context.Context, _ string, w *domain.Widget) (*domain.Widget, error) {
	stored := *w
	stored.ID = "widget-001"
	f.saved = append(f.saved, &stored)
	return &stored, nil
}

func (f *fakeWidgetRepo) GetByID(_ context.Context, _, _ string) (*domain.Widget, error) {
	return f.getResult, f.getErr
}

func (f *fakeWidgetRepo) PublishIfPublishable(_ context.Context, _, _ string) (*domain.Widget, error) {
	return f.pubResult, f.pubErr
}

func (f *fakeWidgetRepo) ListByTenant(_ context.Context, _ string, _, _ int) ([]*domain.Widget, int, error) {
	return nil, 0, nil
}

// ---------------------------------------------------------------------------
// Behavior 1: Create
// ---------------------------------------------------------------------------

func TestWidgetService_Create_ValidName_PersistsDraft(t *testing.T) {
	repo := &fakeWidgetRepo{}
	svc := NewWidgetService(repo)

	w, err := svc.Create(context.Background(), "tenant-001", "hero banner")

	require.NoError(t, err)
	require.NotNil(t, w)
	assert.Equal(t, domain.WidgetStatusDraft, w.Status)
	require.Len(t, repo.saved, 1, "exactly one widget must be written")
}

func TestWidgetService_Create_EmptyName_ReturnsValidationErrorAndNoDBWrite(t *testing.T) {
	repo := &fakeWidgetRepo{}
	svc := NewWidgetService(repo)

	_, err := svc.Create(context.Background(), "tenant-001", "")

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
	require.Len(t, repo.saved, 0, "no widget may be written on validation failure")
}

// ---------------------------------------------------------------------------
// Behavior 2: Get
// ---------------------------------------------------------------------------

func TestWidgetService_Get_RepoReturnsNotFound_PassesErrorThrough(t *testing.T) {
	repo := &fakeWidgetRepo{getErr: apperrors.ErrNotFound}
	svc := NewWidgetService(repo)

	_, err := svc.Get(context.Background(), "tenant-001", "missing")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Behavior 3: Publish delegates to the repository
// ---------------------------------------------------------------------------

func TestWidgetService_Publish_Succeeds_ReturnsPersistedWidget(t *testing.T) {
	persisted := &domain.Widget{ID: "widget-001", TenantID: "tenant-001", Name: "hero banner", Status: domain.WidgetStatusPublished}
	repo := &fakeWidgetRepo{pubResult: persisted}
	svc := NewWidgetService(repo)

	w, err := svc.Publish(context.Background(), "tenant-001", "widget-001")

	require.NoError(t, err)
	assert.Equal(t, domain.WidgetStatusPublished, w.Status)
	assert.Equal(t, "widget-001", w.ID)
}

func TestWidgetService_Publish_RepoRejectsTransition_PassesErrorThrough(t *testing.T) {
	repo := &fakeWidgetRepo{pubErr: apperrors.ErrConflict}
	svc := NewWidgetService(repo)

	_, err := svc.Publish(context.Background(), "tenant-001", "widget-001")

	require.ErrorIs(t, err, apperrors.ErrConflict)
}
