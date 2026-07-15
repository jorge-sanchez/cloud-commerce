# ADR-007: Admin UI as a React SPA served from a Go container

**Status**: Accepted
**Date**: 2026-07-15

## Context

Phase 1 ends with a minimal admin UI (issue #5): sign-up, sign-in, store
settings, staff, products, and stock without curl. The backend surface it
consumes already exists — JSON APIs with JWT auth, wire shapes owned by
each service's `apitypes.go`.

Constraints: one bootstrapping founder (ADR-003/-005 economics apply: no
always-on cost, no new deploy machinery); the catalog admin needs real
interactivity (variant matrix editing, later stock dashboards and POS);
CLAUDE.md already anticipates generating TypeScript types from the Go wire
structs.

## Decision

We will build the admin as a **Vite + React + TypeScript SPA**, with API
types **generated from the Go `apitypes.go` structs via tygo** — the
backend stays the single owner of every wire shape. It deploys as
`services/admin`: a tiny Go binary embedding the built assets
(`embed.FS`), shipped through the existing Dockerfile/deploy workflow to
Cloud Run with scale-to-zero. The SPA calls the service APIs directly
(CORS) with the merchant JWT held in memory/localStorage.

## Alternatives considered

- **Next.js (SSR)** — server rendering buys SEO and first-paint for public
  pages; an authenticated admin has neither need. It would also add a
  Node runtime service to operate.
- **Server-rendered Go templates + HTMX** — the leanest option and
  tempting for a Go codebase, but the variant matrix, stock views, and the
  eventual POS push hard toward client-side state; committing to it now
  invites a rewrite mid-Phase 2.
- **Static hosting (Cloud Storage + CDN)** — cheaper per request than a
  container, but introduces a second deploy path and cache invalidation
  story; the embedded-Go approach reuses the exact pipeline every other
  service uses, at scale-to-zero cost.
- **An admin template/framework (Refine, React-Admin)** — faster CRUD
  scaffolding, but generic data-provider abstractions fight our typed,
  intent-named APIs; plain React with generated types stays closer to the
  domain.

## Consequences

Easier: one deploy pipeline for everything; wire-shape drift between Go
and TypeScript becomes a compile error instead of a runtime bug; the SPA
skills/components transfer to the Phase 2 storefront and later POS.

Harder: CORS configuration on every service (explicit allowed origin, not
`*`); JWTs in the browser demand care (short TTLs already per ADR-006;
refresh tokens become the first follow-up when sessions feel too short);
a `package.json` toolchain enters a Go monorepo — CI gets a Node job
scoped to `services/admin`.

Follow-up work:

- `services/admin` scaffold (Vite + React + TS, embedded in Go) and a
  tygo generation step with a CI check that generated types are current.
- CORS middleware in `pkg/` wired into merchants, catalog, and inventory.
- Auth flow: login against merchants, token in memory, 401 → redirect.

Revisit triggers: a public storefront needing SEO (that's a separate app,
likely SSR — do not stretch this SPA); refresh-token work lands in
merchants when admin sessions need longevity.
