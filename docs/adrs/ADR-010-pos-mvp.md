# ADR-010: POS as an offline-queued admin surface, cash first

**Status**: Accepted
**Date**: 2026-07-16

## Context

The vision's third pillar: in-person sales syncing with online stock in
real time. Reservations (#37) are in. A POS client must survive flaky
store connectivity, and staff already authenticate via platform JWTs.

## Decision

POS ships as a **section of the existing admin SPA** (same auth, types,
deploy) — a dedicated device app is a later concern. Sales are **cash
first**: a POS sale is an order created *already paid* through a
merchant-authed endpoint (`POST /v1/pos/sales`), emitting `order_paid`
so stock deducts through the existing pipeline (no reservation — the
fallback deduction is exactly right for instant sales).

**Offline tolerance is client-side**: sales queue in localStorage with a
**client-generated sale ID**, flushed when connectivity returns; the
endpoint is idempotent on that ID (UNIQUE column), so replays return the
original order instead of double-charging stock.

## Alternatives considered

- **Separate POS app** — a third frontend for one founder; the admin
  already has auth, tokens, and deploy. Revisit for dedicated hardware.
- **Stripe Terminal now** — card-present hardware and SDK before any
  merchant asked; cash proves the flow, Terminal slots behind the same
  endpoint later.
- **Offline via service-worker + IndexedDB** — heavier machinery than a
  localStorage queue needs at MVP; upgrade when catalogs must be
  browsable fully offline.

## Consequences

Easier: reuses everything; sync-safe by construction (idempotency key);
stock, receipts (buyer email optional), and analytics work unchanged
because a POS sale is just a paid order.

Harder: catalog browsing needs connectivity at MVP (only *submission*
queues offline); no cash-drawer/receipt-printer integration; refunds of
cash sales mark state but return no money (gateway skips POS references).

Revisit triggers: Stripe Terminal request; a merchant needing full
offline catalog; dedicated POS hardware.
