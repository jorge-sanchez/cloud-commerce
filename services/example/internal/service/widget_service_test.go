// Test Budget: 4 distinct behaviors × 2 = 8 max unit tests
// Actual: 6
//
// Behavior 1: Create — persists a draft widget; empty name is a validation error
// Behavior 2: Get — delegates to repo and passes errors through
// Behavior 3: Publish — returns the persisted widget and emits the event
// Behavior 4: Publish — event-publish failure is not surfaced to the caller
package service

import (
	"context"
	"errors"
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

var _ EventPublisher = (*fakePublisher)(nil)

type fakePublisher struct {
	err    error
	events []domain.WidgetPublishedEvent
}

func (f *fakePublisher) PublishWidgetPublished(_ context.Context, e domain.WidgetPublishedEvent) error {
	f.events = append(f.events, e)
	return f.err
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
// Behavior 3: Publish emits event built from the persisted widget
// ---------------------------------------------------------------------------

func TestWidgetService_Publish_Succeeds_EmitsEventFromPersistedWidget(t *testing.T) {
	persisted := &domain.Widget{ID: "widget-001", TenantID: "tenant-001", Name: "hero banner", Status: domain.WidgetStatusPublished}
	repo := &fakeWidgetRepo{pubResult: persisted}
	pub := &fakePublisher{}
	svc := NewWidgetService(repo, WithEventPublisher(pub))

	w, err := svc.Publish(context.Background(), "tenant-001", "widget-001")

	require.NoError(t, err)
	assert.Equal(t, domain.WidgetStatusPublished, w.Status)
	require.Len(t, pub.events, 1, "exactly one event must be emitted")
	assert.Equal(t, "widget-001", pub.events[0].WidgetID)
}

func TestWidgetService_Publish_RepoRejectsTransition_ReturnsErrorAndNoEvent(t *testing.T) {
	repo := &fakeWidgetRepo{pubErr: apperrors.ErrConflict}
	pub := &fakePublisher{}
	svc := NewWidgetService(repo, WithEventPublisher(pub))

	_, err := svc.Publish(context.Background(), "tenant-001", "widget-001")

	require.ErrorIs(t, err, apperrors.ErrConflict)
	require.Len(t, pub.events, 0, "no event may be emitted when the transition is rejected")
}

// ---------------------------------------------------------------------------
// Behavior 4: event-publish failure is swallowed once the row is persisted
// ---------------------------------------------------------------------------

func TestWidgetService_Publish_EventPublishFails_StillReturnsWidget(t *testing.T) {
	persisted := &domain.Widget{ID: "widget-001", TenantID: "tenant-001", Status: domain.WidgetStatusPublished}
	repo := &fakeWidgetRepo{pubResult: persisted}
	pub := &fakePublisher{err: errors.New("broker down")}
	svc := NewWidgetService(repo, WithEventPublisher(pub))

	w, err := svc.Publish(context.Background(), "tenant-001", "widget-001")

	require.NoError(t, err, "publish failure must not be surfaced once the row is persisted")
	assert.Equal(t, "widget-001", w.ID)
}
