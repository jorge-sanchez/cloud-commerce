# RFC-001: Shipping addresses, methods, and costs at checkout

**Status**: Accepted (2026-07-16)
**Author(s)**: Claude (with Jorge Sanchez)
**Date**: 2026-07-16
**Related**: Discussion #47 (Tier 1, item 1); ADR-008 (provider-port
pattern); ADR-009 (storefront); will spawn ADR-011 (rate source) on
acceptance.

## Summary

Orders today capture only a buyer email — there is no address, no
shipping choice, and no shipping cost. This RFC adds merchant-defined
shipping methods, buyer address capture at checkout, and shipping as a
priced line on the order, end to end from storefront to fulfillment
email. It is the data-model foundation the rest of launch readiness
(taxes, discounts) builds on, which is why it goes first.

## Motivation

A store that cannot ask where to send the goods cannot ship them. Every
Tier-1 launch feature that touches money (taxes, discounts) composes
with order totals, so the totals model must learn "lines beyond items"
exactly once — here.

## Design

**Rates are merchant-defined flat methods** (launch scope): the merchant
configures methods — name, price, active — in store settings. Carrier-
calculated rates arrive later behind a `RateProvider` port (the ADR-008
pattern: design the seam now, integrate on demand). This becomes ADR-011.

**Ownership**: the merchants service owns shipping-method configuration
(it is store settings, beside currency/timezone): owner-only CRUD at
`/v1/shipping-methods`, and the methods exposed on the public store
lookup so buyer surfaces need no extra call. Orders never stores config.

**Address model**: structured snapshot on the order — recipient name,
line1, line2, city, region, postal code, ISO country, phone. Stored like
prices are stored: what was true at checkout, immutable afterwards. No
address book until buyer accounts. Validation is entity-owned and
minimal (required fields, country shape); no verification service.

**Checkout contract**: `POST /v1/public/carts/:id/checkout` gains
`shipping_address` and `shipping_method_id`. Orders resolves the method
via the merchants public API (as it already resolves the store),
snapshots its name/price onto the order, and the entity computes
`total = items + shipping`. Order responses, admin views, and receipt/
shipping emails show the shipping line and address; `order_placed`/
`order_paid` events carry both so consumers never look them up.

**Storefront**: the cart page grows an address form and method selector
(prices shown) before the payment step.

**Out of scope**: carrier rate APIs and label purchase; zone-based
rates (all methods apply store-wide at launch; zones ride on the same
table later); address verification; picking up Tier-2 discounts.

## Alternatives considered

- **Carrier-calculated rates first (Shippo/EasyPost/USPS)** — real
  accuracy, but an external dependency, API keys, and per-request cost
  before a single merchant asked; flat rates are what small stores use
  anyway. The port seam preserves the upgrade.
- **Orders service owns method config** — keeps checkout self-contained
  but plants store settings in the wrong aggregate; merchants already
  owns settings and the public lookup.
- **Free-form address text blob** — trivially flexible, permanently
  unqueryable; structured fields cost little now and unlock carrier
  APIs, tax jurisdictions, and analytics later.
- **A shipping microservice** — nothing here has independent scale or
  lifecycle; the fewer-coarse-services rule applies.

## Rollout

1. Merchants: `shipping_methods` table + CRUD + public exposure.
2. Orders: address/method columns, checkout contract, entity totals,
   events enriched (additive — v1 stability holds).
3. Storefront + admin + emails render the new data.
4. Existing orders predate shipping: address columns nullable, old
   rows display "no shipping details" — no backfill.
5. Live verification: browser purchase with address + method, shipping
   visible in admin, email, and events.

## Open questions — resolved at acceptance

- **≥1 active method required to check out**: yes; the migration seeds
  a free "Standard" method for existing tenants.
- **POS sales**: null address, method recorded as "in-store", and —
  raised in review — the sale carries a **`location_id`** so multi-
  location merchants know which register sold what: the POS screen
  gets a per-device location selector, the order stores the location,
  `order_paid` carries it, and inventory deducts at *that* location
  instead of the default (tenant scoping makes foreign IDs inert).
  Online orders keep location null; fulfillment-location routing is a
  future feature.
