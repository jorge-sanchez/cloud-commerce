# ADR-011: Merchant-defined flat shipping rates behind a future rate port

**Status**: Accepted
**Date**: 2026-07-16

## Context

RFC-001 (accepted) adds shipping to checkout. The load-bearing decision
is where rates come from: merchant configuration or carrier APIs.

## Decision

Launch with **merchant-defined flat methods** (name + price, owner-
managed in store settings, exposed on the public store lookup). Carrier-
calculated rates arrive later behind a `RateProvider` port — the ADR-008
pattern: the seam is designed now, the integration waits for a merchant
who needs it.

## Alternatives considered

- **Carrier APIs first (Shippo/EasyPost/USPS)** — accuracy before
  anyone asked, at the cost of an external dependency, keys, and
  per-request fees; small stores use flat rates anyway.
- **Zone-based rates now** — zones ride on the same table later;
  starting store-wide keeps the launch surface small.

## Consequences

Easier: shipping ships without a third-party account; totals learn
"lines beyond items" once, which taxes and discounts reuse.
Harder: international/heavy items are mispriced until zones or
carriers arrive — merchants compensate with method naming ("US only").

Revisit triggers: a merchant requests carrier rates or zones; label
purchase (Phase 3 deferral) would share the provider.
