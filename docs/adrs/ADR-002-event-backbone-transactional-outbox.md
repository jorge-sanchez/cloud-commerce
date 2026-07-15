# ADR-002: Transactional outbox as the event backbone

**Status**: Accepted
**Date**: 2026-07-15

## Context

Services communicate state changes through domain events (`WidgetPublished`
in the reference service; `OrderPaid`, `StockReserved`, … in real services).
Two rules are already fixed by CLAUDE.md and must be honored by whatever
backbone we choose:

1. A state change and its event must not be able to diverge: once the row is
   committed, the event **will** be delivered eventually.
2. Publish failures are not surfaced to the caller when the row is already
   persisted — "a recovery process owns retries." That recovery process does
   not exist yet; this ADR defines it.

Writing to the database and publishing to a broker are two systems — there
is no transaction spanning both (the dual-write problem). Constraints: a
small team, no existing broker infrastructure, Postgres already deployed per
service, and low event volume for the foreseeable future.

## Decision

We will use a transactional outbox: each service writes domain events to its
own `outbox` table in the same transaction as the state change, and a
per-service relay drains the table in insertion order, delivering with
at-least-once semantics and marking rows delivered. Repositories record
events through an `EventRecorder` port (implemented by the producer
package's outbox recorder) inside their own transaction — recording moved to
the repository layer because that is the only place the state-change
transaction exists. The relay's initial transport is direct Postgres-backed
delivery (consumers read from the destination the relay writes); adopting a
broker later replaces only the relay's delivery step.

## Alternatives considered

- **Publish directly to a broker in the request path** — the dual-write
  problem: a crash between commit and publish silently loses the event,
  violating rule 1. This is exactly the failure mode the CLAUDE.md recovery
  rule exists to absorb.
- **Kafka (or NATS JetStream) from day one** — solves transport, not
  atomicity: an outbox (or CDC) is still required to bridge the database and
  the broker. Standing up and operating a broker now buys ordering and
  fan-out we don't yet need, at a permanent operational cost.
- **CDC / Debezium reading the WAL** — the heavyweight cousin of the outbox:
  no double-write in application code, but it brings Kafka Connect–class
  infrastructure and WAL-format coupling. Right answer at high volume, wrong
  answer for a small team pre-launch.
- **Synchronous HTTP between services** — couples availability (a consumer
  outage fails the producer's request), provides no replay, and inverts the
  dependency direction the domain events are meant to preserve.

## Consequences

Easier: exactly the semantics CLAUDE.md promises — commit-then-deliver with
retries owned by the relay; local development and CI need only Postgres;
events are replayable from the outbox table for backfills and debugging.

Harder: delivery is at-least-once, so **every consumer must be idempotent**
(dedupe on event ID or use natural idempotency keys). Ordering is guaranteed
only per outbox (per service, insertion order), not globally. The relay is a
new runtime component per service and needs monitoring — outbox lag (oldest
undelivered row age) is the metric that pages.

Follow-up work:

- Add an `outbox` table migration and a relay implementation to the service
  template, wired behind the producer port, so new services inherit it.
- Define the event envelope (event ID, tenant ID, aggregate ID, type,
  occurred-at, payload) in a shared `pkg/` module.
- Add outbox-lag metrics and alerting when observability lands (Phase 0).

Revisit trigger: when cross-service fan-out (several consumers per event) or
throughput makes per-service Postgres delivery a bottleneck, introduce a
broker as the relay's transport. Domain code, producer ports, and the outbox
itself do not change — only the relay's delivery step.

*Amended 2026-07-15:* with Google Cloud as the platform (ADR-003), that
broker will be **Google Pub/Sub** — a `Deliverer` implementation publishing
to a topic. Pub/Sub is zero-ops and pay-per-use with a permanent free tier,
so the swap is gated on need (a real second consumer), not cost. The outbox
stays either way: no broker solves dual-write atomicity.
