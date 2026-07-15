// Package repository holds the persistence adapters. Repositories load and
// save — the entity decides. No business logic lives here.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"

	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/pkg/events"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/domain"
)

// EventRecorder writes an event envelope inside the caller's transaction —
// the transactional-outbox port (ADR-002). Implemented by outbox.Recorder.
type EventRecorder interface {
	Record(ctx context.Context, tx *sql.Tx, env events.Envelope) error
}

// PostgresMerchantRepository implements domain.MerchantRepository on PostgreSQL.
type PostgresMerchantRepository struct {
	db     *sql.DB       // required
	events EventRecorder // may be nil
}

var _ domain.MerchantRepository = (*PostgresMerchantRepository)(nil)

// Option configures optional dependencies on the repository.
type Option func(*PostgresMerchantRepository)

// WithEventRecorder wires the outbox recorder. Without it, state changes
// persist but no events are recorded.
func WithEventRecorder(rec EventRecorder) Option {
	return func(r *PostgresMerchantRepository) { r.events = rec }
}

// NewPostgresMerchantRepository wires the repository to an open *sql.DB.
func NewPostgresMerchantRepository(db *sql.DB, opts ...Option) *PostgresMerchantRepository {
	r := &PostgresMerchantRepository{db: db}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

const uniqueViolation = "23505"

func (r *PostgresMerchantRepository) SaveNewWithOwner(ctx context.Context, m *domain.Merchant, owner *domain.User) (*domain.Merchant, *domain.User, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	merchant := *m
	err = tx.QueryRowContext(ctx, `
		INSERT INTO merchants (name, status)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at`,
		m.Name, string(m.Status),
	).Scan(&merchant.ID, &merchant.CreatedAt, &merchant.UpdatedAt)
	if err != nil {
		return nil, nil, apperrors.ErrInternal.Wrap(err)
	}

	user := *owner
	user.MerchantID = merchant.ID
	err = tx.QueryRowContext(ctx, `
		INSERT INTO merchant_users (merchant_id, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		user.MerchantID, user.Email, user.PasswordHash, string(user.Role),
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == uniqueViolation {
			return nil, nil, apperrors.ErrConflict.Wrap(err)
		}
		return nil, nil, apperrors.ErrInternal.Wrap(err)
	}

	// Record the event in the same transaction as the aggregate (ADR-002).
	if r.events != nil {
		event := domain.NewMerchantSignedUpEvent(&merchant, &user, time.Now().UTC())
		env, err := events.New(merchant.ID, merchant.ID, domain.MerchantSignedUpEventType, event.SignedUpAt, event)
		if err != nil {
			return nil, nil, apperrors.ErrInternal.Wrap(err)
		}
		if err := r.events.Record(ctx, tx, env); err != nil {
			return nil, nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, apperrors.ErrInternal.Wrap(err)
	}
	return &merchant, &user, nil
}

func (r *PostgresMerchantRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.scanUser(r.db.QueryRowContext(ctx, `
		SELECT id, merchant_id, email, password_hash, role, created_at
		FROM merchant_users WHERE email = $1`,
		email,
	))
}

func (r *PostgresMerchantRepository) GetMerchantWithUser(ctx context.Context, tenantID, userID string) (*domain.Merchant, *domain.User, error) {
	user, err := r.scanUser(r.db.QueryRowContext(ctx, `
		SELECT id, merchant_id, email, password_hash, role, created_at
		FROM merchant_users WHERE merchant_id = $1 AND id = $2`,
		tenantID, userID,
	))
	if err != nil {
		return nil, nil, err
	}

	var m domain.Merchant
	var status string
	err = r.db.QueryRowContext(ctx, `
		SELECT id, name, status, created_at, updated_at
		FROM merchants WHERE id = $1`,
		tenantID,
	).Scan(&m.ID, &m.Name, &status, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, nil, apperrors.ErrInternal.Wrap(err)
	}
	m.Status = domain.MerchantStatus(status)
	return &m, user, nil
}

func (r *PostgresMerchantRepository) scanUser(row *sql.Row) (*domain.User, error) {
	var u domain.User
	var role string
	err := row.Scan(&u.ID, &u.MerchantID, &u.Email, &u.PasswordHash, &role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	u.Role = domain.UserRole(role)
	return &u, nil
}
