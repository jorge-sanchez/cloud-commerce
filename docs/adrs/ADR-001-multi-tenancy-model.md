# ADR-001: Shared Postgres schema with tenant_id for multi-tenancy

**Status**: Accepted
**Date**: 2026-07-15

## Context

Cloud Commerce is a SaaS platform: every merchant (tenant) shares the same
deployed services, but no tenant may ever see another tenant's data. The
tenancy model decides how data is physically laid out in Postgres, and it is
effectively irreversible once real services exist — every table, query,
index, migration, and backup strategy depends on it.

Constraints:

- A small team operates the platform; per-tenant infrastructure work must be
  near zero, because onboarding a merchant has to be a row insert, not a
  provisioning job.
- We expect many small tenants long before we see any large ones.
- The reference service (`services/example`) already scopes every repository
  method by a `tenantID` first argument and carries a `tenant_id` column on
  its tables, so the codebase has a de-facto pattern awaiting ratification.

## Decision

We will use shared databases with shared tables, discriminated by a
`tenant_id` column on every tenant-owned row. Every repository method takes
`tenantID` as its first argument and every query filters on it; no service
code path may touch tenant data without a tenant in hand.

## Alternatives considered

- **Schema-per-tenant** — one Postgres schema per merchant. Isolation is
  better, but migrations must run N times, connection pooling degrades as
  schemas multiply, and cross-tenant platform queries (billing, analytics,
  abuse detection) become painful. Operationally too expensive for a small
  team with many small tenants.
- **Database-per-tenant** — strongest isolation and per-tenant
  restore/export, but onboarding becomes a provisioning workflow and the
  operational surface (backups, migrations, monitoring × N databases) is far
  beyond what the team can carry. This is a premium *tier*, not a default.
- **Shared tables + Postgres row-level security as the primary enforcement**
  — RLS is attractive as defense-in-depth, but making it the *primary*
  mechanism ties every connection to a per-request `SET` of the tenant,
  complicates pooling (pgbouncer transaction mode), and hides the tenancy
  rule from the code reviewer. We prefer tenancy explicit in every query,
  with RLS available as a later hardening layer on top of — not instead of —
  the application rule.

## Consequences

Easier: onboarding is an insert; one migration run per service; platform-wide
queries are ordinary SQL; local development and CI need one database.

Harder: nothing isolates a forgotten `WHERE tenant_id = $1` — the application
convention is the security boundary. Mitigations: repositories are the only
layer that touches SQL, `tenantID` is always the first argument (existing
convention from the example service), and integration tests must include a
cross-tenant negative case for every repository (attempt to read another
tenant's row and expect `ErrNotFound`).

Follow-up work:

- Add the cross-tenant negative test requirement to CLAUDE.md's test rules.
- Composite indexes lead with `tenant_id`.
- Evaluate enabling Postgres RLS as defense-in-depth once the first real
  service is in production.

Revisit triggers: a tenant with compliance requirements (data residency,
dedicated infrastructure) or a tenant large enough to be a noisy neighbor.
The expected evolution is a dedicated-database *tier* for such tenants
layered on the same repository interfaces — not a migration of the default.
