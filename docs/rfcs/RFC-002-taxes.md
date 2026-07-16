# RFC-002: Taxes — multi-region from day one

**Status**: Accepted (2026-07-16)
**Author(s)**: Claude (with Jorge Sanchez)
**Date**: 2026-07-16
**Related**: Reviewed as Discussion #54 (see resolutions comment);
RFC-001 (shipping/totals model); ADR-008/-011 (provider ports); spawns
ADR-012. Full design text in Discussion #54; this file is the canonical
record of the accepted design.

## Summary

Tax joins checkout and price presentation, designed for the regulatory
split across our future regions: **tax-inclusive pricing** (EU VAT,
Peru IGV, most of South America) versus **tax-exclusive pricing**
(US/CA, added at checkout). US launches first; the model must survive
region two without reinterpreting stored prices.

## Design (accepted)

1. **Per-store `tax_mode`** (`inclusive`/`exclusive`), chosen at store
   creation, near-immutable once orders exist. Inclusive: totals
   unchanged, tax extracted for display (`price × r/(1+r)`).
   Exclusive: `total = items + shipping + tax`.
2. **`TaxCalculator` port (ADR-012)**: merchant-defined jurisdiction
   rates at launch (country + optional region, matched against the
   RFC-001 shipping address; POS taxes at the register's location; no
   match = 0% with an admin warning). Stripe Tax adapter later.
3. **Order tax snapshot**: `tax_cents`, `tax_name`, `tax_rate_bps`,
   `tax_inclusive` — history stays interpretable forever. Rounding:
   half-up on the order total, one rule everywhere.
4. **Surfaces**: storefront shows a tax line (exclusive) or
   "Includes <name> (<rate>)" copy (inclusive); receipts always show
   the tax line; admin gets rates CRUD; events gain tax fields
   (additive).

## Resolutions (review round, Discussion #54)

- Per-rate **"applies to shipping"** boolean, default true.
- **Store country field added at signup**; `tax_mode` defaulted from
  it but confirmed explicitly during onboarding.
- Receipt tax line renders from the merchant's rate name ("Incluye
  IGV (18%)") — no region-specific code paths.

## Out of scope (recorded)

Product tax categories (nullable `tax_category` column reserved now,
unused); B2B tax IDs and EU reverse charge; tax filing/remittance;
**SUNAT electronic invoicing (boletas/facturas)**; multi-currency.

## Rollout

Merchants (country, tax_mode, rates) → orders (snapshot, both-mode
entity math, events) → storefront + receipts → live verification of
BOTH modes (a TX exclusive store and the IGV inclusive store).
