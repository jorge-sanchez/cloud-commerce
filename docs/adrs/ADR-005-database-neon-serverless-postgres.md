# ADR-005: Neon serverless Postgres as the starting database

**Status**: Accepted
**Date**: 2026-07-15

## Context

ADR-003 deferred the database choice, noting that the database — not
compute — would dominate the bill: every Cloud Run service scales to zero,
but a provisioned database bills around the clock. The first deploy is now
blocked on this decision (the deploy workflow expects a
`<service>-database-url` secret).

Constraints: bootstrap budget where the ideal idle cost is zero; the whole
stack speaks plain Postgres DSNs (`lib/pq`, golang-migrate, `pkg/testdb`),
so nothing ties us to any particular Postgres host; pre-launch traffic is
near zero but the platform should still respond whenever someone hits it.

An explicit idea to evaluate: run a managed instance only during work hours
(scheduled stop/start) to cut cost.

## Decision

We will start on **Neon** (serverless Postgres, free tier): one project in
an AWS region close to `us-central1`, autosuspend enabled. Databases per
service follow the one-database-per-service rule from the architecture;
Neon branches or separate databases within the project keep them apart.
Connection strings live in Secret Manager (`<service>-database-url`,
ADR-004 convention) and reach services through the existing `DATABASE_URL`
contract — adopting or leaving Neon is a DSN swap.

Idle cost control is **autosuspend, not a schedule**: Neon suspends compute
after a few minutes of inactivity and wakes on the next connection in
under a second. This delivers what a work-hours schedule would — paying
only for hours actually used — without taking the platform down at night:
a scheduled-off database means a dead API for any beta user, webhook, or
evening demo, and requires scheduling machinery we'd have to build and
watch.

## Alternatives considered

- **Cloud SQL (smallest instance), always on** — the "proper" GCP answer,
  ~$10–25/month before storage; that is more than the entire rest of the
  platform combined at this stage, for capacity we would not use.
- **Cloud SQL, scheduled on/off during work hours** — cheaper (~$3–9/month
  compute for a 45-hour week) but strictly worse than autosuspend: platform
  down outside the window, Cloud Scheduler + permissions to maintain, and
  still storage cost. Rejected in favor of scale-to-zero that we don't
  have to operate.
- **Supabase free tier** — comparable serverless Postgres, but free
  projects pause after a week of inactivity and need manual restore, and
  it bundles an auth/API platform we'd deliberately not use (our services
  own their APIs).
- **Postgres on a free-tier e2-micro VM** — $0 but self-operated: backups,
  patching, disk-full incidents, and data-loss risk on the platform's
  system of record. Founder time and merchant data are the two things we
  must not spend here.

## Consequences

Easier: idle cost is $0 and low-usage cost stays $0 (free tier: ~0.5 GB
storage, ~190 compute-hours/month — far beyond pre-launch needs); no
scheduling machinery; branching gives free throwaway databases for
experiments.

Harder: the database lives outside GCP — cross-cloud latency (~10–30 ms
from `us-central1` to a nearby AWS region) and small egress charges are
acceptable at low volume but are the first thing to re-measure at real
traffic; cold connections after suspend add ~0.5–1 s to the first request
(indistinguishable from a Cloud Run cold start pre-launch); `sslmode=require`
is mandatory in DSNs; the 0.5 GB storage ceiling is a hard trigger, not a
surprise, so watch it.

Follow-up work:

- Create the Neon project and per-service connection strings; store them
  as `<service>-database-url` in Secret Manager (values out-of-band per
  ADR-004 — never through Terraform).
- First deploy of the example service to prove the pipeline end-to-end.

Revisit triggers: storage approaching 0.5 GB, sustained traffic consuming
the free compute allowance, a paying-merchant SLA, or data-residency
requirements — at which point the candidates are Neon's paid tier (same
DSN, no migration) or Cloud SQL (in-GCP latency, `pg_dump`/restore
migration during a maintenance window).
