# ADR-012: Merchant-defined jurisdiction rates behind a TaxCalculator port

**Status**: Accepted
**Date**: 2026-07-16

## Context

RFC-002 (accepted, Discussion #54) adds multi-region tax. The discrete
decision: where tax rates come from.

## Decision

Launch with **merchant-defined jurisdiction rates** (country + optional
region, per-rate shipping-taxability flag) behind a **`TaxCalculator`
port** — the third use of the provider-port pattern (ADR-008 payments,
ADR-011 shipping). **Stripe Tax** becomes an adapter later for US
nexus/district complexity, opt-in per store, never the foundation.

## Alternatives considered

- **Stripe Tax from day one** — couples all regions to one provider's
  coverage and 0.5%/txn pricing, and answers calculation but not the
  inclusive-display question, which is presentation.
- **Built-in jurisdiction rulebook** — a tax-content maintenance burden
  we are structurally unable to keep correct.

## Consequences

Easier: no external dependency or per-transaction fee at launch; PE/EU
single-rate regimes are fully served. Harder: US merchants with multi-
state nexus must maintain their own rates until the Stripe Tax adapter
exists — acceptable for launch-scale merchants, and the port makes the
upgrade additive.

Revisit trigger: a US merchant with nexus beyond one or two states.
