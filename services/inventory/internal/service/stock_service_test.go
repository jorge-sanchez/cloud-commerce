// Test Budget: 5 distinct behaviors × 2 = 10 max unit tests
// Actual: 8
//
// Behavior 1: ProcessEvent(product_created) — ensures the default location
//
//	and initializes zero-stock rows; malformed payloads are validation errors
//
// Behavior 2: ProcessEvent — unknown event types are acked (nil) untouched
// Behavior 3: AdjustStock/ListStock — delegate to repo and pass errors through
// Behavior 4: CreateLocation — entity-rejected name is a validation error
//
//	with no write
//
// Behavior 5: ProcessEvent(order_paid) — deductions reach the repository
//
//	deduped by envelope ID
package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/inventory/internal/domain"
)

// ---------------------------------------------------------------------------
// Hand-rolled fakes at the port boundaries — no gomock, no testify/mock.
// ---------------------------------------------------------------------------

var _ domain.StockRepository = (*fakeStockRepo)(nil)

type fakeStockRepo struct {
	deducted       []domain.StockDeduction
	deductTenant   string
	deductEventID  string
	initialized    []domain.StockInit
	initTenant     string
	initLocation   string
	savedLocations []*domain.Location
	err            error
}

func (f *fakeStockRepo) EnsureDefaultLocation(_ context.Context, tenantID string) (*domain.Location, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &domain.Location{ID: "loc-001", TenantID: tenantID, Name: domain.DefaultLocationName, IsDefault: true}, nil
}

func (f *fakeStockRepo) SaveNewLocation(_ context.Context, tenantID string, l *domain.Location) (*domain.Location, error) {
	stored := *l
	stored.ID = "loc-002"
	stored.TenantID = tenantID
	f.savedLocations = append(f.savedLocations, &stored)
	return &stored, f.err
}

func (f *fakeStockRepo) ListLocations(_ context.Context, _ string) ([]*domain.Location, error) {
	return nil, f.err
}

func (f *fakeStockRepo) InitializeStock(_ context.Context, tenantID, locationID string, items []domain.StockInit) error {
	f.initTenant = tenantID
	f.initLocation = locationID
	f.initialized = append(f.initialized, items...)
	return f.err
}

func (f *fakeStockRepo) ListStockByTenant(_ context.Context, _ string, _, _ int) ([]*domain.StockLevel, int, error) {
	return nil, 0, f.err
}

func (f *fakeStockRepo) AdjustIfSufficient(_ context.Context, _, _, _ string, _ int64) (*domain.StockLevel, error) {
	return nil, f.err
}

func (f *fakeStockRepo) ApplyStockDeduction(_ context.Context, tenantID, eventID string, items []domain.StockDeduction) error {
	f.deductTenant = tenantID
	f.deductEventID = eventID
	f.deducted = append(f.deducted, items...)
	return f.err
}

func productCreatedEnvelope(t *testing.T) events.Envelope {
	t.Helper()
	payload := map[string]any{
		"product_id": "product-001",
		"variants": []map[string]any{
			{"variant_id": "var-001", "sku": "TS-S"},
			{"variant_id": "var-002", "sku": "TS-M"},
		},
	}
	env, err := events.New("tenant-001", "product-001", CatalogProductCreatedType, time.Now(), payload)
	require.NoError(t, err)
	return env
}

// ---------------------------------------------------------------------------
// Behavior 1: product_created initializes stock at the default location
// ---------------------------------------------------------------------------

func TestStockService_ProcessEvent_ProductCreated_InitializesStockAtDefaultLocation(t *testing.T) {
	repo := &fakeStockRepo{}
	svc := NewStockService(repo)

	err := svc.ProcessEvent(context.Background(), productCreatedEnvelope(t))

	require.NoError(t, err)
	assert.Equal(t, "tenant-001", repo.initTenant)
	assert.Equal(t, "loc-001", repo.initLocation, "stock must land at the default location")
	require.Len(t, repo.initialized, 2, "one zero-stock row per variant must be initialized")
	assert.Equal(t, "TS-S", repo.initialized[0].SKU)
}

func TestStockService_ProcessEvent_MalformedPayload_ReturnsValidationError(t *testing.T) {
	repo := &fakeStockRepo{}
	svc := NewStockService(repo)
	env := productCreatedEnvelope(t)
	env.Payload = json.RawMessage(`"not-an-object"`)

	err := svc.ProcessEvent(context.Background(), env)

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
}

// ---------------------------------------------------------------------------
// Behavior 2: unknown event types are acked untouched
// ---------------------------------------------------------------------------

func TestStockService_ProcessEvent_UnknownType_AcksWithoutTouchingRepo(t *testing.T) {
	repo := &fakeStockRepo{}
	svc := NewStockService(repo)
	env := productCreatedEnvelope(t)
	env.Type = "catalog.product_activated"

	err := svc.ProcessEvent(context.Background(), env)

	require.NoError(t, err, "unknown types must be acked, not retried forever")
	require.Len(t, repo.initialized, 0, "unknown types must not touch the repository")
}

// ---------------------------------------------------------------------------
// Behavior 3: delegation passes errors through
// ---------------------------------------------------------------------------

func TestStockService_AdjustStock_RepoRejects_PassesConflictThrough(t *testing.T) {
	svc := NewStockService(&fakeStockRepo{err: apperrors.ErrConflict})

	_, err := svc.AdjustStock(context.Background(), "tenant-001", "loc-001", "var-001", -5)

	require.ErrorIs(t, err, apperrors.ErrConflict)
}

func TestStockService_ListStock_RepoFails_PassesErrorThrough(t *testing.T) {
	svc := NewStockService(&fakeStockRepo{err: apperrors.ErrInternal})

	_, _, err := svc.ListStock(context.Background(), "tenant-001", 1, 20)

	require.ErrorIs(t, err, apperrors.ErrInternal)
}

// ---------------------------------------------------------------------------
// Behavior 4: CreateLocation validates through the entity
// ---------------------------------------------------------------------------

func TestStockService_CreateLocation_ValidName_Persists(t *testing.T) {
	repo := &fakeStockRepo{}
	svc := NewStockService(repo)

	location, err := svc.CreateLocation(context.Background(), "tenant-001", "Almacén Central")

	require.NoError(t, err)
	assert.False(t, location.IsDefault, "explicitly created locations are never the default")
	require.Len(t, repo.savedLocations, 1, "exactly one location must be written")
}

func TestStockService_CreateLocation_EmptyName_ReturnsValidationErrorAndNoWrite(t *testing.T) {
	repo := &fakeStockRepo{}
	svc := NewStockService(repo)

	_, err := svc.CreateLocation(context.Background(), "tenant-001", "   ")

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
	require.Len(t, repo.savedLocations, 0, "no location may be written on validation failure")
}

// ---------------------------------------------------------------------------
// Behavior 5: order_paid deducts stock deduped by envelope ID
// ---------------------------------------------------------------------------

func TestStockService_ProcessEvent_OrderPaid_DeductsDedupedByEnvelopeID(t *testing.T) {
	repo := &fakeStockRepo{}
	svc := NewStockService(repo)
	payload := map[string]any{
		"order_id": "order-001",
		"items":    []map[string]any{{"variant_id": "var-001", "qty": 2}},
	}
	env, err := events.New("tenant-001", "order-001", OrdersOrderPaidType, time.Now(), payload)
	require.NoError(t, err)
	env.ID = "envelope-001"

	require.NoError(t, svc.ProcessEvent(context.Background(), env))

	assert.Equal(t, "tenant-001", repo.deductTenant)
	assert.Equal(t, "envelope-001", repo.deductEventID, "dedupe must key on the envelope ID")
	require.Len(t, repo.deducted, 1, "one deduction per order line")
	assert.Equal(t, int64(2), repo.deducted[0].Qty)
}
