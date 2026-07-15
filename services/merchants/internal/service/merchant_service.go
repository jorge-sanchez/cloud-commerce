// Package service holds the application services: orchestration only, no
// business rules. Required dependencies are positional constructor arguments;
// optional dependencies are Option functions.
package service

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/domain"
)

// TokenIssuer is the token-minting port, satisfied by *auth.Issuer. Only
// this service issues platform identity (ADR-006).
type TokenIssuer interface {
	Issue(c auth.Claims) (string, error)
}

// Session is an authenticated identity plus its bearer token.
type Session struct {
	Token    string
	Merchant *domain.Merchant
	User     *domain.User
}

// MerchantService is the application-service port consumed by the handlers.
type MerchantService interface {
	SignUp(ctx context.Context, storeName, email, password string) (*Session, error)
	LogIn(ctx context.Context, email, password string) (*Session, error)
	Me(ctx context.Context, tenantID, userID string) (*domain.Merchant, *domain.User, error)
	GetStore(ctx context.Context, tenantID string) (*domain.Merchant, error)
	UpdateStore(ctx context.Context, tenantID string, actorRole domain.UserRole, name string, settings domain.StoreSettings) (*domain.Merchant, error)
	AddStaff(ctx context.Context, tenantID string, actorRole domain.UserRole, email, password string) (*domain.User, error)
	ListStaff(ctx context.Context, tenantID string, actorRole domain.UserRole) ([]*domain.User, error)
	RemoveStaff(ctx context.Context, tenantID string, actorRole domain.UserRole, userID string) error
}

type merchantService struct {
	repo   domain.MerchantRepository // required
	issuer TokenIssuer               // required
}

// Option configures optional dependencies on the merchant service.
type Option func(*merchantService)

// NewMerchantService constructs the merchant application service.
func NewMerchantService(repo domain.MerchantRepository, issuer TokenIssuer, opts ...Option) MerchantService {
	s := &merchantService{repo: repo, issuer: issuer}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *merchantService) SignUp(ctx context.Context, storeName, email, password string) (*Session, error) {
	merchant, err := domain.NewMerchant(storeName)
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	if err := domain.ValidatePassword(password); err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	owner, err := domain.NewOwner(email, string(hash))
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}

	savedMerchant, savedOwner, err := s.repo.SaveNewWithOwner(ctx, merchant, owner)
	if err != nil {
		return nil, err
	}
	return s.session(savedMerchant, savedOwner)
}

func (s *merchantService) LogIn(ctx context.Context, email, password string) (*Session, error) {
	normalized, err := domain.NormalizeEmail(email)
	if err != nil {
		return nil, apperrors.ErrUnauthorized
	}

	user, err := s.repo.GetUserByEmail(ctx, normalized)
	if err != nil {
		// Unknown email and wrong password are indistinguishable on the
		// wire — no account enumeration.
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, apperrors.ErrUnauthorized
		}
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, apperrors.ErrUnauthorized
	}

	merchant, _, err := s.repo.GetMerchantWithUser(ctx, user.MerchantID, user.ID)
	if err != nil {
		return nil, err
	}
	return s.session(merchant, user)
}

func (s *merchantService) Me(ctx context.Context, tenantID, userID string) (*domain.Merchant, *domain.User, error) {
	return s.repo.GetMerchantWithUser(ctx, tenantID, userID)
}

func (s *merchantService) GetStore(ctx context.Context, tenantID string) (*domain.Merchant, error) {
	return s.repo.GetByID(ctx, tenantID)
}

func (s *merchantService) UpdateStore(ctx context.Context, tenantID string, actorRole domain.UserRole, name string, settings domain.StoreSettings) (*domain.Merchant, error) {
	if !actorRole.CanManageStaff() {
		return nil, apperrors.ErrForbidden
	}
	// The entity validates inside the repository transaction; the settings
	// event is recorded there too (ADR-002).
	return s.repo.UpdateStoreProfile(ctx, tenantID, name, settings)
}

func (s *merchantService) AddStaff(ctx context.Context, tenantID string, actorRole domain.UserRole, email, password string) (*domain.User, error) {
	if !actorRole.CanManageStaff() {
		return nil, apperrors.ErrForbidden
	}
	if err := domain.ValidatePassword(password); err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	staff, err := domain.NewStaff(email, string(hash))
	if err != nil {
		return nil, apperrors.ErrValidation.Wrap(err)
	}
	return s.repo.SaveNewStaff(ctx, tenantID, staff)
}

func (s *merchantService) ListStaff(ctx context.Context, tenantID string, actorRole domain.UserRole) ([]*domain.User, error) {
	if !actorRole.CanManageStaff() {
		return nil, apperrors.ErrForbidden
	}
	return s.repo.ListUsers(ctx, tenantID)
}

func (s *merchantService) RemoveStaff(ctx context.Context, tenantID string, actorRole domain.UserRole, userID string) error {
	if !actorRole.CanManageStaff() {
		return apperrors.ErrForbidden
	}
	return s.repo.DeleteUserIfRemovable(ctx, tenantID, userID)
}

func (s *merchantService) session(m *domain.Merchant, u *domain.User) (*Session, error) {
	token, err := s.issuer.Issue(auth.Claims{UserID: u.ID, TenantID: m.ID, Email: u.Email, Role: string(u.Role)})
	if err != nil {
		return nil, apperrors.ErrInternal.Wrap(err)
	}
	return &Session{Token: token, Merchant: m, User: u}, nil
}
