// Test Budget: 4 distinct behaviors × 2 = 8 max unit tests
// Actual: 8
//
// Behavior 1: Issue + Verify — round-trips claims; expired tokens are rejected
// Behavior 2: Verify — foreign-key and tampered tokens are rejected
// Behavior 3: Key construction — malformed keys fail with ErrBadKey
// Behavior 4: Middleware — valid token injects identity; missing token is 401
package auth_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jorge-sanchez/cloud-commerce/pkg/auth"
)

func testKeypair(t *testing.T) (string, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(priv.Seed()), base64.StdEncoding.EncodeToString(pub)
}

func testClaims() auth.Claims {
	return auth.Claims{UserID: "user-001", TenantID: "tenant-001", Email: "owner@store.test"}
}

// ---------------------------------------------------------------------------
// Behavior 1: Issue + Verify round-trip
// ---------------------------------------------------------------------------

func TestIssuerVerifier_IssueThenVerify_RoundTripsClaims(t *testing.T) {
	seed, pub := testKeypair(t)
	issuer, err := auth.NewIssuer(seed)
	require.NoError(t, err)
	verifier, err := auth.NewVerifier(pub)
	require.NoError(t, err)

	token, err := issuer.Issue(testClaims())
	require.NoError(t, err)

	claims, err := verifier.Verify(token)
	require.NoError(t, err)
	assert.Equal(t, "user-001", claims.UserID)
	assert.Equal(t, "tenant-001", claims.TenantID)
	assert.Equal(t, "owner@store.test", claims.Email)
}

func TestVerifier_ExpiredToken_ReturnsErrInvalidToken(t *testing.T) {
	seed, pub := testKeypair(t)
	issuer, err := auth.NewIssuer(seed, auth.WithTTL(-time.Minute))
	require.NoError(t, err)
	verifier, err := auth.NewVerifier(pub)
	require.NoError(t, err)

	token, err := issuer.Issue(testClaims())
	require.NoError(t, err)

	_, err = verifier.Verify(token)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

// ---------------------------------------------------------------------------
// Behavior 2: foreign and tampered tokens are rejected
// ---------------------------------------------------------------------------

func TestVerifier_TokenFromDifferentKey_ReturnsErrInvalidToken(t *testing.T) {
	foreignSeed, _ := testKeypair(t)
	_, pub := testKeypair(t)
	foreignIssuer, err := auth.NewIssuer(foreignSeed)
	require.NoError(t, err)
	verifier, err := auth.NewVerifier(pub)
	require.NoError(t, err)

	token, err := foreignIssuer.Issue(testClaims())
	require.NoError(t, err)

	_, err = verifier.Verify(token)
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

func TestVerifier_GarbageToken_ReturnsErrInvalidToken(t *testing.T) {
	_, pub := testKeypair(t)
	verifier, err := auth.NewVerifier(pub)
	require.NoError(t, err)

	_, err = verifier.Verify("not.a.token")
	require.ErrorIs(t, err, auth.ErrInvalidToken)
}

// ---------------------------------------------------------------------------
// Behavior 3: malformed keys
// ---------------------------------------------------------------------------

func TestNewIssuer_MalformedSeed_ReturnsErrBadKey(t *testing.T) {
	_, err := auth.NewIssuer("definitely-not-base64!")
	require.ErrorIs(t, err, auth.ErrBadKey)
}

func TestNewVerifier_WrongLengthKey_ReturnsErrBadKey(t *testing.T) {
	_, err := auth.NewVerifier(base64.StdEncoding.EncodeToString([]byte("short")))
	require.ErrorIs(t, err, auth.ErrBadKey)
}

// ---------------------------------------------------------------------------
// Behavior 4: middleware injects identity or rejects
// ---------------------------------------------------------------------------

func middlewareRequest(t *testing.T, pub, authorization string) (*httptest.ResponseRecorder, *auth.Claims) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	verifier, err := auth.NewVerifier(pub)
	require.NoError(t, err)

	var seen *auth.Claims
	router := gin.New()
	router.GET("/probe", auth.Middleware(verifier), func(c *gin.Context) {
		seen = &auth.Claims{UserID: auth.UserID(c), TenantID: auth.TenantID(c), Email: auth.Email(c)}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec, seen
}

func TestMiddleware_ValidToken_InjectsIdentityIntoContext(t *testing.T) {
	seed, pub := testKeypair(t)
	issuer, err := auth.NewIssuer(seed)
	require.NoError(t, err)
	token, err := issuer.Issue(testClaims())
	require.NoError(t, err)

	rec, seen := middlewareRequest(t, pub, "Bearer "+token)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.NotNil(t, seen, "the handler must run for a valid token")
	assert.Equal(t, "tenant-001", seen.TenantID)
	assert.Equal(t, "user-001", seen.UserID)
}

func TestMiddleware_MissingToken_RejectsWith401(t *testing.T) {
	_, pub := testKeypair(t)

	rec, seen := middlewareRequest(t, pub, "")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, seen, "the handler must not run without a token")
}
