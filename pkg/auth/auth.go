// Package auth is the shared identity contract (ADR-006): Ed25519-signed
// JWTs issued by the merchants service and verified statelessly by every
// service. The Gin middleware resolves the tenant from verified claims —
// handlers must read it via TenantID(c), never from a header.
package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuerName = "cloud-commerce"

// Sentinel errors. Middleware maps ErrInvalidToken to a 401; key errors are
// startup failures.
var (
	ErrInvalidToken = errors.New("token is missing, malformed, expired, or not signed by the platform")
	ErrBadKey       = errors.New("key must be a base64-encoded 32-byte Ed25519 key")
)

// Claims is the platform identity carried by every token. Role is the
// merchant-user role ("owner" or "staff"); tokens issued before roles
// existed carry an empty role and hold no owner privileges.
type Claims struct {
	UserID   string
	TenantID string
	Email    string
	Role     string
}

type jwtClaims struct {
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Issuer mints tokens. Only the merchants service holds a private key.
type Issuer struct {
	key ed25519.PrivateKey
	ttl time.Duration
}

// IssuerOption configures optional issuer behavior.
type IssuerOption func(*Issuer)

// WithTTL overrides the default 1-hour token lifetime.
func WithTTL(d time.Duration) IssuerOption {
	return func(i *Issuer) { i.ttl = d }
}

// NewIssuer builds an issuer from a base64-encoded 32-byte Ed25519 seed.
func NewIssuer(privateKeySeedBase64 string, opts ...IssuerOption) (*Issuer, error) {
	seed, err := base64.StdEncoding.DecodeString(privateKeySeedBase64)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%w: private seed", ErrBadKey)
	}
	i := &Issuer{key: ed25519.NewKeyFromSeed(seed), ttl: time.Hour}
	for _, opt := range opts {
		opt(i)
	}
	return i, nil
}

// Issue signs a token for the given identity.
func (i *Issuer) Issue(c Claims) (string, error) {
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwtClaims{
		TenantID: c.TenantID,
		Email:    c.Email,
		Role:     c.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   c.UserID,
			Issuer:    issuerName,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.ttl)),
		},
	})
	return token.SignedString(i.key)
}

// Verifier validates tokens with the platform public key.
type Verifier struct {
	key ed25519.PublicKey
}

// NewVerifier builds a verifier from a base64-encoded 32-byte public key.
func NewVerifier(publicKeyBase64 string) (*Verifier, error) {
	raw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: public key", ErrBadKey)
	}
	return &Verifier{key: ed25519.PublicKey(raw)}, nil
}

// Verify parses and validates a compact token and returns its claims.
func (v *Verifier) Verify(tokenString string) (Claims, error) {
	var parsed jwtClaims
	_, err := jwt.ParseWithClaims(tokenString, &parsed,
		func(t *jwt.Token) (any, error) { return v.key, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodEdDSA.Alg()}),
		jwt.WithIssuer(issuerName),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if parsed.TenantID == "" || parsed.Subject == "" {
		return Claims{}, fmt.Errorf("%w: identity claims missing", ErrInvalidToken)
	}
	return Claims{UserID: parsed.Subject, TenantID: parsed.TenantID, Email: parsed.Email, Role: parsed.Role}, nil
}

// BearerToken extracts the compact token from an Authorization header value.
func BearerToken(header string) (string, bool) {
	token := strings.TrimPrefix(header, "Bearer ")
	return token, token != "" && token != header
}
