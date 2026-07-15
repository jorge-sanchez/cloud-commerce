// Package repository holds the persistence adapters. Repositories load and
// save — the entity decides. No business logic lives here.
package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	insert := func(handle string) error {
		merchant.Handle = handle
		return tx.QueryRowContext(ctx, `
			INSERT INTO merchants (name, handle, status, currency, timezone, support_email)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at, updated_at`,
			m.Name, handle, string(m.Status), m.Settings.Currency, m.Settings.Timezone, m.Settings.SupportEmail,
		).Scan(&merchant.ID, &merchant.CreatedAt, &merchant.UpdatedAt)
	}
	err = insert(m.Handle)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == uniqueViolation {
		// Handle taken by another store: retry once with a random suffix.
		err = insert(m.Handle + "-" + randomSuffix())
	}
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

	merchant, err := r.GetByID(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	return merchant, user, nil
}

const merchantColumns = `id, name, handle, status, currency, timezone, support_email, created_at, updated_at`

func (r *PostgresMerchantRepository) GetByID(ctx context.Context, tenantID string) (*domain.Merchant, error) {
	return r.scanMerchant(r.db.QueryRowContext(ctx,
		`SELECT `+merchantColumns+` FROM merchants WHERE id = $1`, tenantID))
}

func (r *PostgresMerchantRepository) GetByHandle(ctx context.Context, handle string) (*domain.Merchant, error) {
	return r.scanMerchant(r.db.QueryRowContext(ctx,
		`SELECT `+merchantColumns+` FROM merchants WHERE handle = $1`, handle))
}

// randomSuffix returns four hex characters for handle collision retries.
func randomSuffix() string {
	var b [2]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// UpdateStoreProfile loads the merchant inside a transaction, lets the
// entity validate the new profile, and persists what the entity decided.
func (r *PostgresMerchantRepository) UpdateStoreProfile(ctx context.Context, tenantID, name string, settings domain.StoreSettings) (*domain.Merchant, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	merchant, err := r.scanMerchant(tx.QueryRowContext(ctx,
		`SELECT `+merchantColumns+` FROM merchants WHERE id = $1 FOR UPDATE`, tenantID))
	if err != nil {
		return nil, err
	}

	if err := merchant.UpdateProfile(name, settings); err != nil { // entity decides
		return nil, apperrors.ErrValidation.Wrap(err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE merchants
		SET name = $1, currency = $2, timezone = $3, support_email = $4, updated_at = NOW()
		WHERE id = $5`,
		merchant.Name, merchant.Settings.Currency, merchant.Settings.Timezone,
		merchant.Settings.SupportEmail, tenantID,
	); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}

	// Record the event in the same transaction as the state change (ADR-002).
	if r.events != nil {
		event := domain.NewMerchantSettingsUpdatedEvent(merchant, time.Now().UTC())
		env, err := events.New(merchant.ID, merchant.ID, domain.MerchantSettingsUpdatedEventType, event.UpdatedAt, event)
		if err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		if err := r.events.Record(ctx, tx, env); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return merchant, nil
}

func (r *PostgresMerchantRepository) scanMerchant(row *sql.Row) (*domain.Merchant, error) {
	var m domain.Merchant
	var status string
	err := row.Scan(&m.ID, &m.Name, &m.Handle, &status, &m.Settings.Currency, &m.Settings.Timezone,
		&m.Settings.SupportEmail, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	m.Status = domain.MerchantStatus(status)
	return &m, nil
}

func (r *PostgresMerchantRepository) SaveNewStaff(ctx context.Context, tenantID string, u *domain.User) (*domain.User, error) {
	user := *u
	user.MerchantID = tenantID
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO merchant_users (merchant_id, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		tenantID, user.Email, user.PasswordHash, string(user.Role),
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == uniqueViolation {
			return nil, apperrors.ErrConflict.Wrap(err)
		}
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &user, nil
}

func (r *PostgresMerchantRepository) ListUsers(ctx context.Context, tenantID string) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, merchant_id, email, password_hash, role, created_at
		FROM merchant_users WHERE merchant_id = $1
		ORDER BY role = 'owner' DESC, created_at`,
		tenantID,
	)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = rows.Close() }()

	users := make([]*domain.User, 0, 8)
	for rows.Next() {
		var u domain.User
		var role string
		if err := rows.Scan(&u.ID, &u.MerchantID, &u.Email, &u.PasswordHash, &role, &u.CreatedAt); err != nil {
			return nil, apperrors.ErrInternal.Wrap(err)
		}
		u.Role = domain.UserRole(role)
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return users, nil
}

// DeleteUserIfRemovable loads the user inside a transaction, lets the entity
// decide whether it may be removed, and deletes what the entity allowed.
func (r *PostgresMerchantRepository) DeleteUserIfRemovable(ctx context.Context, tenantID, userID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	defer func() { _ = tx.Rollback() }()

	user, err := r.scanUser(tx.QueryRowContext(ctx, `
		SELECT id, merchant_id, email, password_hash, role, created_at
		FROM merchant_users WHERE merchant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, userID,
	))
	if err != nil {
		return err
	}

	if !user.CanBeRemoved() { // entity decides
		return apperrors.ErrConflict.Wrap(errors.New("the owner cannot be removed"))
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM merchant_users WHERE merchant_id = $1 AND id = $2`,
		tenantID, userID,
	); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	if err := tx.Commit(); err != nil {
		return apperrors.ErrInternal.Wrap(err)
	}
	return nil
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
