// Package domain holds the Merchant aggregate: the merchant (which IS the
// tenant, ADR-006), its users, domain events, and the repository interface.
// Business rules live here — services orchestrate, repositories persist.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MerchantStatus is the lifecycle state of a merchant account.
type MerchantStatus string

const (
	MerchantStatusActive    MerchantStatus = "active"
	MerchantStatusSuspended MerchantStatus = "suspended"
)

// UserRole is a merchant user's role. Owner is created at sign-up; staff
// comes with Phase 1 store settings.
type UserRole string

const (
	UserRoleOwner UserRole = "owner"
	UserRoleStaff UserRole = "staff"
)

// Domain sentinel errors for entity-level failures.
var (
	ErrInvalidEmail    = errors.New("email address is not valid")
	ErrWeakPassword    = errors.New("password must be at least 8 characters")
	ErrEmptyName       = errors.New("store name must not be empty")
	ErrNotSuspendable  = errors.New("merchant cannot be suspended in its current status")
	ErrInvalidCurrency = errors.New("currency must be a three-letter ISO 4217 code")
	ErrInvalidTimezone = errors.New("timezone must be a valid IANA zone name")
)

// StoreSettings is the merchant-configurable store profile.
type StoreSettings struct {
	Currency     string // ISO 4217, e.g. "USD"
	Timezone     string // IANA zone, e.g. "America/Lima"
	SupportEmail string // shown to buyers; may be empty
}

// DefaultStoreSettings are applied at sign-up.
func DefaultStoreSettings() StoreSettings {
	return StoreSettings{Currency: "USD", Timezone: "UTC"}
}

// Merchant is the aggregate root. Its ID is the platform tenant ID. Handle
// is the public storefront URL key — globally unique, since it resolves
// the tenant.
type Merchant struct {
	ID        string
	Name      string
	Handle    string
	Status    MerchantStatus
	Settings  StoreSettings
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SlugifyHandle derives a storefront handle: lowercase, non-alphanumerics
// collapsed to single hyphens.
func SlugifyHandle(name string) string {
	var b strings.Builder
	lastHyphen := true // suppress leading hyphen
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// UpdateProfile applies a new name and settings. The entity validates —
// callers must not write Name or Settings directly.
func (m *Merchant) UpdateProfile(name string, s StoreSettings) error {
	if strings.TrimSpace(name) == "" {
		return ErrEmptyName
	}
	if len(s.Currency) != 3 || strings.ToUpper(s.Currency) != s.Currency {
		return fmt.Errorf("%w: %q", ErrInvalidCurrency, s.Currency)
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "" {
		return fmt.Errorf("%w: %q", ErrInvalidTimezone, s.Timezone)
	}
	if s.SupportEmail != "" {
		normalized, err := NormalizeEmail(s.SupportEmail)
		if err != nil {
			return err
		}
		s.SupportEmail = normalized
	}
	m.Name = strings.TrimSpace(name)
	m.Settings = s
	return nil
}

// User is a person who can sign in to a merchant account.
type User struct {
	ID           string
	MerchantID   string
	Email        string
	PasswordHash string
	Role         UserRole
	CreatedAt    time.Time
}

// NewMerchant constructs an active merchant with default settings. ID and
// timestamps are assigned by the repository on save.
func NewMerchant(name string) (*Merchant, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyName
	}
	m := &Merchant{
		Name:     strings.TrimSpace(name),
		Status:   MerchantStatusActive,
		Settings: DefaultStoreSettings(),
	}
	m.Handle = SlugifyHandle(m.Name)
	if m.Handle == "" {
		return nil, ErrEmptyName
	}
	return m, nil
}

// NewOwner constructs the owner user for a new merchant. The caller supplies
// the already-hashed password — plaintext never enters the domain.
func NewOwner(email, passwordHash string) (*User, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return &User{Email: normalized, PasswordHash: passwordHash, Role: UserRoleOwner}, nil
}

// NewStaff constructs a staff user for an existing merchant.
func NewStaff(email, passwordHash string) (*User, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	return &User{Email: normalized, PasswordHash: passwordHash, Role: UserRoleStaff}, nil
}

// CanManageStaff reports whether a user with this role may add or remove
// staff and change store settings.
func (r UserRole) CanManageStaff() bool {
	return r == UserRoleOwner
}

// CanBeRemoved reports whether this user may be deleted. The owner is
// immutable — a merchant always has exactly one.
func (u *User) CanBeRemoved() bool {
	return u.Role != UserRoleOwner
}

// NormalizeEmail lowercases and validates an email address.
func NormalizeEmail(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	at := strings.Index(normalized, "@")
	if at < 1 || at == len(normalized)-1 || !strings.Contains(normalized[at:], ".") {
		return "", fmt.Errorf("%w: %q", ErrInvalidEmail, email)
	}
	return normalized, nil
}

// ValidatePassword enforces the platform password rule.
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}
	return nil
}

// CanSuspend reports whether the merchant may transition to suspended.
func (m *Merchant) CanSuspend() bool {
	return m.Status == MerchantStatusActive
}

// Suspend transitions the merchant to suspended. The entity decides its own
// transitions — callers must not check or set Status directly.
func (m *Merchant) Suspend() error {
	if !m.CanSuspend() {
		return fmt.Errorf("%w: status is %q", ErrNotSuspendable, m.Status)
	}
	m.Status = MerchantStatusSuspended
	return nil
}

// MerchantSignedUpEventType is the envelope type for MerchantSignedUpEvent.
const MerchantSignedUpEventType = "merchant.signed_up"

// MerchantSignedUpEvent is emitted when a merchant account is created.
type MerchantSignedUpEvent struct {
	MerchantID string    `json:"merchant_id"`
	Name       string    `json:"name"`
	OwnerEmail string    `json:"owner_email"`
	SignedUpAt time.Time `json:"signed_up_at"`
}

// NewMerchantSignedUpEvent builds the event from the persisted aggregate.
func NewMerchantSignedUpEvent(m *Merchant, owner *User, at time.Time) MerchantSignedUpEvent {
	return MerchantSignedUpEvent{
		MerchantID: m.ID,
		Name:       m.Name,
		OwnerEmail: owner.Email,
		SignedUpAt: at,
	}
}

// MerchantSettingsUpdatedEventType is the envelope type for
// MerchantSettingsUpdatedEvent.
const MerchantSettingsUpdatedEventType = "merchant.settings_updated"

// MerchantSettingsUpdatedEvent is emitted when the store profile changes;
// storefront and catalog caches invalidate on it.
type MerchantSettingsUpdatedEvent struct {
	MerchantID string    `json:"merchant_id"`
	Name       string    `json:"name"`
	Currency   string    `json:"currency"`
	Timezone   string    `json:"timezone"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// NewMerchantSettingsUpdatedEvent builds the event from the persisted aggregate.
func NewMerchantSettingsUpdatedEvent(m *Merchant, at time.Time) MerchantSettingsUpdatedEvent {
	return MerchantSettingsUpdatedEvent{
		MerchantID: m.ID,
		Name:       m.Name,
		Currency:   m.Settings.Currency,
		Timezone:   m.Settings.Timezone,
		UpdatedAt:  at,
	}
}

// MerchantRepository is the persistence port for the Merchant aggregate.
type MerchantRepository interface {
	// SaveNewWithOwner persists the merchant and its owner user atomically
	// (aggregate rule: no separate user insert). A taken email returns
	// apperrors.ErrConflict.
	SaveNewWithOwner(ctx context.Context, m *Merchant, owner *User) (*Merchant, *User, error)
	// GetUserByEmail returns the user for login, or apperrors.ErrNotFound.
	// Deliberately pre-tenant: authentication is how a tenant is discovered.
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	// GetMerchantWithUser returns the merchant and the requesting user,
	// tenant-scoped, or apperrors.ErrNotFound.
	GetMerchantWithUser(ctx context.Context, tenantID, userID string) (*Merchant, *User, error)
	// GetByID returns the merchant, tenant-scoped (id IS the tenant), or
	// apperrors.ErrNotFound.
	GetByID(ctx context.Context, tenantID string) (*Merchant, error)
	// GetByHandle resolves a public storefront handle. Deliberately
	// pre-tenant: the handle is how a buyer discovers the tenant.
	GetByHandle(ctx context.Context, handle string) (*Merchant, error)
	// UpdateStoreProfile loads the merchant, lets the entity validate the
	// new profile, and persists what the entity decided. Returns
	// apperrors.ErrValidation when the entity rejects it.
	UpdateStoreProfile(ctx context.Context, tenantID, name string, settings StoreSettings) (*Merchant, error)
	// SaveNewStaff persists a staff user for an existing merchant. A taken
	// email returns apperrors.ErrConflict.
	SaveNewStaff(ctx context.Context, tenantID string, u *User) (*User, error)
	// ListUsers returns all users of the merchant, owner first.
	ListUsers(ctx context.Context, tenantID string) ([]*User, error)
	// DeleteUserIfRemovable loads the user, lets the entity decide whether
	// it may be removed (the owner may not), and deletes it. Returns
	// apperrors.ErrConflict when the entity refuses.
	DeleteUserIfRemovable(ctx context.Context, tenantID, userID string) error
}
