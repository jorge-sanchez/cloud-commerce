# ADR-006: JWT identity issued by the merchants service, tenant from claims

**Status**: Accepted
**Date**: 2026-07-15

## Context

Every service so far trusts an `X-Tenant-ID` header — a placeholder that
must die before any real service ships, because the tenant filter is the
tenancy security boundary (ADR-001) and today the caller picks the tenant.
Phase 0 requires merchant accounts and a tenant-resolution middleware every
service reuses.

Constraints: Cloud Run scale-to-zero (per-request DB session lookups would
also keep Neon awake, ADR-005); services must verify identity without
calling another service on every request; solo-maintainable; the merchant →
staff-user model is core commerce domain (Phase 1 already needs store
settings and staff roles).

## Decision

The **merchants service owns identity**: merchant sign-up creates the
merchant (the tenant) and its owner user in one transaction; login verifies
bcrypt-hashed credentials and issues a short-lived **JWT signed with
Ed25519**. Only the merchants service holds the private key (Secret
Manager); every service verifies with the public key — no service but
merchants can mint identity.

A shared **`pkg/auth`** module provides the issuer, the verifier, and the
Gin middleware: it validates the Bearer token and injects `tenant_id` and
`user_id` into the request context. Handlers read the tenant exclusively
through `auth.TenantID(c)`. The `X-Tenant-ID` header is removed.

## Alternatives considered

- **GCP Identity Platform / Firebase Auth** — free at our scale and removes
  password handling, but the merchant→staff→roles model is core domain we
  must own in Postgres anyway, so it would add a mapping layer plus vendor
  coupling on the platform's front door. Remains a candidate for *buyer*
  accounts (storefront customers) later, where identity is commodity.
- **Server-side sessions in Postgres** — revocable, but stateful: a DB
  round-trip on every request in every service, cross-service coupling to
  the session store, and constant DB wakeups that defeat Neon autosuspend.
- **HS256 (shared-secret) JWTs** — one leaked secret anywhere lets any
  service mint tokens for any tenant. Asymmetric keys confine minting to
  the identity service at near-zero extra complexity.
- **Per-service accounts** — no. One identity, one tenant claim, resolved
  identically everywhere via `pkg/auth`.

## Consequences

Easier: stateless verification (no network, no DB) in every service; the
tenant can no longer be chosen by the caller; local development mints its
own tokens with a dev keypair (`pkg/auth/cmd/genkey`).

Harder: JWTs are not revocable before expiry — mitigated with short TTLs
(1 hour) and, later, a merchant `status` check at sensitive operations;
key rotation is manual (generate, publish new public key, retire old);
refresh tokens are deliberately deferred until the admin UI needs session
longevity.

Follow-up work:

- Secrets: `jwt-private-key` (merchants only) and `jwt-public-key` (all
  services) in Secret Manager; deploy workflow wires them.
- Buyer/customer identity for storefronts is a separate decision in
  Phase 2 — do not stretch merchant JWTs to cover it.

Revisit triggers: staff roles beyond owner (claims grow a role field);
admin UI sessions (add refresh tokens); any compromise scenario requiring
revocation lists.
