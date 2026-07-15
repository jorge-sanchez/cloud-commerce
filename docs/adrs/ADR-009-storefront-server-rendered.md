# ADR-009: Server-rendered storefront with data-driven templates

**Status**: Accepted
**Date**: 2026-07-15

## Context

Buyers have no web storefront — the platform's public APIs are headless
and proven (cart → checkout → Stripe payment all run in production), but
only the admin has a UI. Phase 2 deferred the "server-rendered starter
theme"; Phase 4's theme builder presupposes it exists. ADR-007 already
drew the line: the admin SPA must not be stretched into a storefront,
because storefronts need SEO and fast first paint on cheap phones.

Constraints: one founder (no second frontend stack to babysit), scale-to-
zero economics (ADR-003), and the theme-builder future — whatever renders
pages must treat layout as data from day one, or Phase 4's editor becomes
a rewrite.

## Decision

We will build `services/storefront` as a **server-rendered Go service**
(`html/template`), one deployment serving every store, resolved by handle
in the path (`/:handle`, `/:handle/p/:productID`). Pages render from the
existing public APIs. Interactivity (cart, checkout, Stripe.js payment)
is a thin vanilla-JS layer over the same public endpoints the headless
API exposes — the storefront is a *client* of the platform, never a
backdoor into it.

Templates are **data-driven from day one**: a per-tenant theme document
(colors, sections, ordering — stored JSON, defaulted centrally) feeds the
templates, so the Phase 4 builder edits data, not code.

## Alternatives considered

- **Next.js/SSR framework** — the industry default for storefronts, but
  it adds a Node runtime service and a second framework for one person to
  own; Go templates on our existing service chassis reuse everything
  (Dockerfile, deploy, logging, CORS-free same-origin calls).
- **Extending the admin SPA** — rejected in ADR-007 for exactly this
  case: no SEO, JS-gated first paint, and tangled auth models (buyers are
  anonymous; merchants are not).
- **Static site generation per store** — fastest pages, but a build-per-
  merchant pipeline is heavy machinery for stores whose catalogs change
  daily; SSR with short cache headers gets 90% of it free.
- **Custom domains now** — deferred: path-based (`/:handle`) ships first;
  domain mapping is additive later (Cloud Run domain mappings + a lookup).

## Consequences

Easier: SEO-renderable pages at scale-to-zero cost; one more Go service
on the exact template every other service uses; theme = data, so the
builder is an editor project as the roadmap intended.

Harder: the payment page needs Stripe.js and the *publishable* key
(pk_test_/pk_live_ — not a secret, but per-account, so it arrives from
the dashboard); server-rendered interactivity discipline (no SPA
creep — if a page needs app-like state, it belongs in a different
surface); per-store caching wants care once traffic exists.

Follow-up work: `services/storefront` per issue #36; theme document
schema kept minimal (it will grow with the builder); custom domains and
image handling remain separate future issues.

Revisit triggers: the theme builder (Phase 4 proper) stressing the
template model; real traffic making per-request rendering costly (add
caching, then consider SSG for hot stores).
