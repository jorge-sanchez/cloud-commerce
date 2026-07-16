// Test Budget: 6 distinct behaviors × 2 = 12 max unit tests
// Actual: 10
//
// Behavior 1: SignUp — persists merchant+owner atomically and returns a
//
//	session; weak password is a validation error with no write
//
// Behavior 2: LogIn — correct credentials return a session with claims from
//
//	the stored identity; wrong password is ErrUnauthorized
//
// Behavior 3: LogIn — unknown email is ErrUnauthorized (no enumeration)
// Behavior 4: Me — delegates to repo tenant-scoped and passes errors through
// Behavior 5: staff management — owner adds staff (hashed, staff role);
//
//	non-owner is ErrForbidden with no write
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
	apperrors "github.com/jorge-sanchez/cloud-commerce/pkg/errors"
	"github.com/jorge-sanchez/cloud-commerce/services/merchants/internal/domain"
)

// ---------------------------------------------------------------------------
// Hand-rolled fakes at the port boundaries — no gomock, no testify/mock.
// ---------------------------------------------------------------------------

var _ domain.MerchantRepository = (*fakeMerchantRepo)(nil)

type fakeMerchantRepo struct {
	savedKeys      []*domain.APIKey
	keyByHash      *domain.APIKey
	savedMerchants []*domain.Merchant
	savedStaff     []*domain.User
	userByEmail    *domain.User
	userByEmailErr error
	merchant       *domain.Merchant
	user           *domain.User
	getErr         error
}

func (f *fakeMerchantRepo) SaveNewWithOwner(_ context.Context, m *domain.Merchant, owner *domain.User) (*domain.Merchant, *domain.User, error) {
	storedM := *m
	storedM.ID = "merchant-001"
	storedU := *owner
	storedU.ID = "user-001"
	storedU.MerchantID = storedM.ID
	f.savedMerchants = append(f.savedMerchants, &storedM)
	return &storedM, &storedU, nil
}

func (f *fakeMerchantRepo) GetUserByEmail(_ context.Context, _ string) (*domain.User, error) {
	return f.userByEmail, f.userByEmailErr
}

func (f *fakeMerchantRepo) GetMerchantWithUser(_ context.Context, _, _ string) (*domain.Merchant, *domain.User, error) {
	return f.merchant, f.user, f.getErr
}

func (f *fakeMerchantRepo) GetByID(_ context.Context, _ string) (*domain.Merchant, error) {
	return f.merchant, f.getErr
}

func (f *fakeMerchantRepo) SaveNewAPIKey(_ context.Context, tenantID, name, _ string) (*domain.APIKey, error) {
	k := &domain.APIKey{ID: "key-001", TenantID: tenantID, Name: name}
	f.savedKeys = append(f.savedKeys, k)
	return k, nil
}

func (f *fakeMerchantRepo) ListAPIKeys(_ context.Context, _ string) ([]*domain.APIKey, error) {
	return f.savedKeys, f.getErr
}

func (f *fakeMerchantRepo) RevokeAPIKey(_ context.Context, _, _ string) error { return f.getErr }

func (f *fakeMerchantRepo) GetAPIKeyByHash(_ context.Context, _ string) (*domain.APIKey, error) {
	if f.keyByHash == nil {
		return nil, apperrors.ErrNotFound
	}
	return f.keyByHash, nil
}

func (f *fakeMerchantRepo) GetByHandle(_ context.Context, _ string) (*domain.Merchant, error) {
	return f.merchant, f.getErr
}

func (f *fakeMerchantRepo) UpdateStoreProfile(_ context.Context, _, _ string, _ domain.StoreSettings) (*domain.Merchant, error) {
	return f.merchant, f.getErr
}

func (f *fakeMerchantRepo) SaveNewStaff(_ context.Context, tenantID string, u *domain.User) (*domain.User, error) {
	stored := *u
	stored.ID = "user-002"
	stored.MerchantID = tenantID
	f.savedStaff = append(f.savedStaff, &stored)
	return &stored, nil
}

func (f *fakeMerchantRepo) ListUsers(_ context.Context, _ string) ([]*domain.User, error) {
	return nil, f.getErr
}

func (f *fakeMerchantRepo) DeleteUserIfRemovable(_ context.Context, _, _ string) error {
	return f.getErr
}

var _ TokenIssuer = (*fakeIssuer)(nil)

type fakeIssuer struct {
	issued []auth.Claims
}

func (f *fakeIssuer) Issue(c auth.Claims) (string, error) {
	f.issued = append(f.issued, c)
	return "token-for-" + c.UserID, nil
}

// ---------------------------------------------------------------------------
// Behavior 1: SignUp
// ---------------------------------------------------------------------------

func TestMerchantService_SignUp_ValidInput_ReturnsSessionWithToken(t *testing.T) {
	repo := &fakeMerchantRepo{}
	issuer := &fakeIssuer{}
	svc := NewMerchantService(repo, issuer)

	session, err := svc.SignUp(context.Background(), "Jorge's Store", "Owner@Store.Test", "correct-horse")

	require.NoError(t, err)
	assert.Equal(t, "token-for-user-001", session.Token)
	assert.Equal(t, domain.MerchantStatusActive, session.Merchant.Status)
	assert.Equal(t, "owner@store.test", session.User.Email, "email must be normalized")
	require.Len(t, repo.savedMerchants, 1, "exactly one merchant must be written")
	require.Len(t, issuer.issued, 1, "exactly one token must be issued")
	assert.Equal(t, "merchant-001", issuer.issued[0].TenantID, "the merchant ID is the tenant claim")
}

func TestMerchantService_SignUp_WeakPassword_ReturnsValidationErrorAndNoWrite(t *testing.T) {
	repo := &fakeMerchantRepo{}
	svc := NewMerchantService(repo, &fakeIssuer{})

	_, err := svc.SignUp(context.Background(), "Jorge's Store", "owner@store.test", "short")

	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
	require.Len(t, repo.savedMerchants, 0, "no merchant may be written on validation failure")
}

// ---------------------------------------------------------------------------
// Behavior 2: LogIn with credentials
// ---------------------------------------------------------------------------

func loginFixture(t *testing.T, password string) *fakeMerchantRepo {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	user := &domain.User{ID: "user-001", MerchantID: "merchant-001", Email: "owner@store.test", PasswordHash: string(hash), Role: domain.UserRoleOwner}
	return &fakeMerchantRepo{
		userByEmail: user,
		merchant:    &domain.Merchant{ID: "merchant-001", Name: "Jorge's Store", Status: domain.MerchantStatusActive},
		user:        user,
	}
}

func TestMerchantService_LogIn_CorrectPassword_ReturnsSessionWithTenantClaim(t *testing.T) {
	repo := loginFixture(t, "correct-horse")
	issuer := &fakeIssuer{}
	svc := NewMerchantService(repo, issuer)

	session, err := svc.LogIn(context.Background(), "owner@store.test", "correct-horse")

	require.NoError(t, err)
	assert.Equal(t, "token-for-user-001", session.Token)
	require.Len(t, issuer.issued, 1, "exactly one token must be issued")
	assert.Equal(t, "merchant-001", issuer.issued[0].TenantID)
}

func TestMerchantService_LogIn_WrongPassword_ReturnsUnauthorizedAndNoToken(t *testing.T) {
	repo := loginFixture(t, "correct-horse")
	issuer := &fakeIssuer{}
	svc := NewMerchantService(repo, issuer)

	_, err := svc.LogIn(context.Background(), "owner@store.test", "wrong-password")

	require.ErrorIs(t, err, apperrors.ErrUnauthorized)
	require.Len(t, issuer.issued, 0, "no token may be issued for bad credentials")
}

// ---------------------------------------------------------------------------
// Behavior 3: unknown email is indistinguishable from wrong password
// ---------------------------------------------------------------------------

func TestMerchantService_LogIn_UnknownEmail_ReturnsUnauthorized(t *testing.T) {
	repo := &fakeMerchantRepo{userByEmailErr: apperrors.ErrNotFound}
	svc := NewMerchantService(repo, &fakeIssuer{})

	_, err := svc.LogIn(context.Background(), "nobody@store.test", "whatever-pass")

	require.ErrorIs(t, err, apperrors.ErrUnauthorized, "unknown email must not be distinguishable from a wrong password")
}

// ---------------------------------------------------------------------------
// Behavior 5: staff management authorization
// ---------------------------------------------------------------------------

func TestMerchantService_AddStaff_ByOwner_SavesStaffWithHashedPassword(t *testing.T) {
	repo := &fakeMerchantRepo{}
	svc := NewMerchantService(repo, &fakeIssuer{})

	staff, err := svc.AddStaff(context.Background(), "merchant-001", domain.UserRoleOwner, "Staff@Store.Test", "a-strong-passphrase")

	require.NoError(t, err)
	assert.Equal(t, domain.UserRoleStaff, staff.Role)
	assert.Equal(t, "staff@store.test", staff.Email, "email must be normalized")
	require.Len(t, repo.savedStaff, 1, "exactly one staff user must be written")
	assert.NotEqual(t, "a-strong-passphrase", repo.savedStaff[0].PasswordHash, "the password must be stored hashed")
}

func TestMerchantService_AddStaff_ByStaff_ReturnsForbiddenAndNoWrite(t *testing.T) {
	repo := &fakeMerchantRepo{}
	svc := NewMerchantService(repo, &fakeIssuer{})

	_, err := svc.AddStaff(context.Background(), "merchant-001", domain.UserRoleStaff, "staff@store.test", "a-strong-passphrase")

	require.ErrorIs(t, err, apperrors.ErrForbidden)
	require.Len(t, repo.savedStaff, 0, "no staff user may be written for a non-owner actor")
}

// ---------------------------------------------------------------------------
// Behavior 6: API keys — owner-only creation; exchange mints the api role
// ---------------------------------------------------------------------------

func TestMerchantService_CreateAPIKey_ByStaff_Forbidden(t *testing.T) {
	repo := &fakeMerchantRepo{}
	svc := NewMerchantService(repo, &fakeIssuer{})

	_, _, err := svc.CreateAPIKey(context.Background(), "merchant-001", domain.UserRoleStaff, "zapier")

	require.ErrorIs(t, err, apperrors.ErrForbidden)
	require.Len(t, repo.savedKeys, 0, "no key may be written for a non-owner actor")
}

func TestMerchantService_ExchangeAPIKey_ValidKey_MintsAPIRoleToken(t *testing.T) {
	issuer := &fakeIssuer{}
	svc := NewMerchantService(&fakeMerchantRepo{
		keyByHash: &domain.APIKey{ID: "key-001", TenantID: "merchant-001"},
	}, issuer)

	token, err := svc.ExchangeAPIKey(context.Background(), "cck_abc")

	require.NoError(t, err)
	assert.NotEmpty(t, token)
	require.Len(t, issuer.issued, 1, "exactly one token must be issued")
	assert.Equal(t, "api", issuer.issued[0].Role, "exchanged tokens must carry the api role, never owner")
	assert.Equal(t, "merchant-001", issuer.issued[0].TenantID)
}

// ---------------------------------------------------------------------------
// Behavior 4: Me delegates tenant-scoped
// ---------------------------------------------------------------------------

func TestMerchantService_Me_RepoReturnsNotFound_PassesErrorThrough(t *testing.T) {
	repo := &fakeMerchantRepo{getErr: apperrors.ErrNotFound}
	svc := NewMerchantService(repo, &fakeIssuer{})

	_, _, err := svc.Me(context.Background(), "tenant-001", "user-001")

	require.ErrorIs(t, err, apperrors.ErrNotFound)
}
