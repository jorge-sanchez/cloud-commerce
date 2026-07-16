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
	// ErrNegativeShipping guards flat-rate creation (RFC-001/ADR-011).
	ErrNegativeShipping = errors.New("shipping price must not be negative")
	ErrBadCountry       = errors.New("country must be a two-letter ISO code")
	ErrBadTaxMode       = errors.New("tax mode must be inclusive or exclusive")
	ErrBadTaxRate       = errors.New("tax rate must be between 0 and 10000 basis points")
)

// TaxMode is how a store's prices relate to tax (RFC-002): inclusive
// (EU/PE — displayed prices contain tax) or exclusive (US/CA — tax is
// added at checkout). Near-immutable once the store has orders.
type TaxMode string

const (
	TaxModeInclusive TaxMode = "inclusive"
	TaxModeExclusive TaxMode = "exclusive"
)

// DefaultTaxModeFor derives the onboarding default from the store
// country (confirmed explicitly during signup, RFC-002 resolution).
func DefaultTaxModeFor(country string) TaxMode {
	if country == "US" || country == "CA" {
		return TaxModeExclusive
	}
	return TaxModeInclusive
}

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
	Country   string  // ISO 3166-1 alpha-2 (RFC-002)
	TaxMode   TaxMode // near-immutable once orders exist
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

// NewMerchant constructs an active merchant with default settings. The
// country drives the tax-mode default; an explicit mode (the onboarding
// confirmation, RFC-002) overrides it. ID and timestamps are assigned by
// the repository on save.
func NewMerchant(name, country string, taxMode TaxMode) (*Merchant, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyName
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(country) != 2 {
		return nil, ErrBadCountry
	}
	if taxMode == "" {
		taxMode = DefaultTaxModeFor(country)
	}
	if taxMode != TaxModeInclusive && taxMode != TaxModeExclusive {
		return nil, ErrBadTaxMode
	}
	m := &Merchant{
		Name:     strings.TrimSpace(name),
		Status:   MerchantStatusActive,
		Country:  country,
		TaxMode:  taxMode,
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

// UserRoleAPI marks tokens minted from API keys: full data access, no
// staff/settings management (CanManageStaff is false).
const UserRoleAPI UserRole = "api"

// APIKey is a third-party credential. The plaintext key exists only in the
// creation response; persistence sees the hash.
type APIKey struct {
	ID        string
	TenantID  string
	Name      string
	CreatedAt time.Time
	RevokedAt *time.Time
}

// Revoked reports whether the key has been revoked.
func (k *APIKey) Revoked() bool { return k.RevokedAt != nil }

// TaxRate is a merchant-defined jurisdiction rate (RFC-002, ADR-012).
type TaxRate struct {
	ID                string
	TenantID          string
	Name              string
	Country           string
	Region            string
	RateBps           int
	AppliesToShipping bool
	CreatedAt         time.Time
}

// NewTaxRate validates a jurisdiction rate. AppliesToShipping defaults
// true (RFC-002 resolution: over-collection is recoverable).
func NewTaxRate(tenantID, name, country, region string, rateBps int, appliesToShipping bool) (*TaxRate, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyName
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(country) != 2 {
		return nil, ErrBadCountry
	}
	if rateBps < 0 || rateBps > 10000 {
		return nil, ErrBadTaxRate
	}
	return &TaxRate{
		TenantID: tenantID, Name: strings.TrimSpace(name), Country: country,
		Region: strings.ToUpper(strings.TrimSpace(region)), RateBps: rateBps,
		AppliesToShipping: appliesToShipping,
	}, nil
}

// ShippingMethod is a merchant-defined flat rate (RFC-001, ADR-011).
type ShippingMethod struct {
	ID         string
	TenantID   string
	Name       string
	PriceCents int64
	Active     bool
	CreatedAt  time.Time
}

// NewShippingMethod validates a flat rate.
func NewShippingMethod(tenantID, name string, priceCents int64) (*ShippingMethod, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyName
	}
	if priceCents < 0 {
		return nil, ErrNegativeShipping
	}
	return &ShippingMethod{TenantID: tenantID, Name: strings.TrimSpace(name), PriceCents: priceCents, Active: true}, nil
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
	// SaveNewAPIKey persists a key hash for the tenant.
	SaveNewAPIKey(ctx context.Context, tenantID, name, keyHash string) (*APIKey, error)
	// ListAPIKeys returns the tenant's keys, newest first.
	ListAPIKeys(ctx context.Context, tenantID string) ([]*APIKey, error)
	// RevokeAPIKey marks a key revoked, tenant-scoped; unknown is ErrNotFound.
	RevokeAPIKey(ctx context.Context, tenantID, keyID string) error
	// SaveNewTaxRate persists a jurisdiction rate for the tenant.
	SaveNewTaxRate(ctx context.Context, r *TaxRate) (*TaxRate, error)
	// ListTaxRates returns the tenant's rates (owner view and the public
	// checkout read are the same list — rates hold no secrets).
	ListTaxRates(ctx context.Context, tenantID string) ([]*TaxRate, error)
	// DeleteTaxRate is tenant-scoped; unknown is ErrNotFound.
	DeleteTaxRate(ctx context.Context, tenantID, id string) error
	// SaveNewShippingMethod persists a flat rate for the tenant.
	SaveNewShippingMethod(ctx context.Context, m *ShippingMethod) (*ShippingMethod, error)
	// ListShippingMethods returns the tenant's methods (all when
	// activeOnly is false — owner view; active only for buyers).
	ListShippingMethods(ctx context.Context, tenantID string, activeOnly bool) ([]*ShippingMethod, error)
	// DeactivateShippingMethod is tenant-scoped; unknown is ErrNotFound.
	DeactivateShippingMethod(ctx context.Context, tenantID, id string) error
	// GetAPIKeyByHash resolves an unrevoked key hash — pre-tenant like
	// GetUserByEmail: the key is how an integration discovers the tenant.
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error)
	// DeleteUserIfRemovable loads the user, lets the entity decide whether
	// it may be removed (the owner may not), and deletes it. Returns
	// apperrors.ErrConflict when the entity refuses.
	DeleteUserIfRemovable(ctx context.Context, tenantID, userID string) error
}
