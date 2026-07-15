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
	ErrInvalidEmail   = errors.New("email address is not valid")
	ErrWeakPassword   = errors.New("password must be at least 8 characters")
	ErrEmptyName      = errors.New("store name must not be empty")
	ErrNotSuspendable = errors.New("merchant cannot be suspended in its current status")
)

// Merchant is the aggregate root. Its ID is the platform tenant ID.
type Merchant struct {
	ID        string
	Name      string
	Status    MerchantStatus
	CreatedAt time.Time
	UpdatedAt time.Time
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

// NewMerchant constructs an active merchant. ID and timestamps are assigned
// by the repository on save.
func NewMerchant(name string) (*Merchant, error) {
	if strings.TrimSpace(name) == "" {
		return nil, ErrEmptyName
	}
	return &Merchant{Name: strings.TrimSpace(name), Status: MerchantStatusActive}, nil
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
}
