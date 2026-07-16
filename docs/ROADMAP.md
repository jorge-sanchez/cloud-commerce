# Cloud Commerce — Big-Picture Roadmap

How we get from an empty monorepo to a full commerce platform. Phases are
ordered by dependency and by time-to-a-sellable-product, not by how exciting
the feature is. Each phase ends with something a merchant can actually use.

---

## Guiding principles

1. **Vertical slices, not horizontal layers.** Never spend a quarter building
   "all the services" with nothing usable. Every phase exits with a working
   end-to-end capability.
2. **Integrate before you build.** Payments, email, shipping labels, and tax
   come from providers behind port interfaces (the same pattern as
   `EventPublisher` in the example service). We replace a provider with an
   in-house service only when volume justifies it — never before.
3. **Few coarse services early.** Each new service costs a deploy pipeline,
   migrations, and dashboards. Split a service only when a boundary is proven
   by real friction, not speculatively.
4. **Multi-tenant from day one.** Every table, endpoint, and event carries
   tenant identity (already the pattern in the example service — `tenantID`
   is the first argument of every repository method). Retrofitting tenancy
   later is the most expensive mistake available to us.

## North-star milestone: "first dollar"

A merchant signs up, creates a store, adds a product, and a real buyer
completes a paid checkout. Everything in phases 0–2 exists to reach this;
anything that doesn't serve it waits.

---

## Phase 0 — Platform foundation

*Mostly done:* monorepo with `go.work`, shared `pkg/` modules, DDD reference
service, CI with lint/test/ratchet checks.

*Remaining before feature work starts:*

- **Identity & access** — done (ADR-006): the merchants service issues
  Ed25519 JWTs at sign-up/login; `pkg/auth` middleware resolves the tenant
  from verified claims in every service. The `X-Tenant-ID` header is gone.
- **Multi-tenancy model (ADR)** — shared Postgres with `tenant_id` vs
  schema-per-tenant. Recommendation: shared tables with `tenant_id`, which is
  what the example service already assumes; revisit only at real scale.
- **Event backbone (ADR)** — decided and built: transactional outbox with a
  relay (ADR-002). Google Pub/Sub becomes the relay's transport when a real
  cross-service consumer exists (ADR-002 amendment).
- **Deployment target** — done: Cloud Run (ADR-003) with Terraform-managed
  infrastructure (ADR-004) and Neon serverless Postgres (ADR-005). The
  example service deploys end-to-end (build → migrate → deploy) via GitHub
  Actions with keyless auth.

**Exit criteria:** a deployed hello-service with auth-gated, tenant-scoped
endpoints and CI/CD to production.

## Phase 1 — Merchant core

The nouns of commerce, before any selling happens.

- **Merchant service** — sign-up, store settings, staff users.
- **Catalog service** — products, variants, collections, images. Product +
  variants is the aggregate-root pattern from CLAUDE.md (`SaveWithVariants`).
- **Inventory service** — stock levels per location. Kept separate from
  catalog because POS (phase 4) will hammer inventory without touching
  catalog.
- **Minimal admin UI** — enough for a merchant to manage a catalog without
  `curl`. Headless API first; the admin frontend consumes the same API we
  will later document publicly.

**Exit criteria:** a merchant can sign up and fully manage a catalog and
stock through the admin UI.

*Status: **done** (2026-07-15) — merchants, catalog, inventory, and the
admin SPA are live on Cloud Run; the event backbone runs on Pub/Sub
(ADR-002 amendment); the admin UI is https://admin-bjm36sbwlq-uc.a.run.app
(ADR-007).*

## Phase 2 — Selling ("first dollar")

- **Cart & checkout service** — cart lifecycle, totals, checkout session.
- **Orders service** — the order state machine (pending → paid → fulfilled →
  …) as a domain entity that owns its transitions.
- **Payments integration** — a `PaymentGateway` port with a provider
  implementation (Stripe or similar). We do **not** process payments
  ourselves: staying out of PCI scope buys us years. Webhook reconciliation
  is part of this deliverable, not an afterthought.
- **Storefront API** — headless JSON API for browsing and buying, plus one
  server-rendered starter theme. The drag-and-drop builder is explicitly
  *not* in this phase.

**Exit criteria:** a buyer completes a paid order end-to-end; the merchant
sees it in the admin; payment webhooks reconcile order state.

*Status: **done** (2026-07-15) — cart → checkout → Stripe test-mode payment
→ order paid → stock decremented, all live on Cloud Run (ADR-008). Webhook
reconciliation is tracked as a follow-up (#25); confirmation is
buyer-initiated polling until then.*

## Phase 3 — Operations

What a merchant needs the week after their first sale.

- **Fulfillment & shipping** — mark fulfilled, shipping labels via provider,
  tracking numbers on orders.
- **Notifications** — transactional email (order confirmation, shipping
  updates) behind a port; provider-backed.
- **Refunds & returns** — through the same payment gateway port.
- **Basic analytics** — sales over time, top products; read models fed by
  domain events (the outbox earns its keep here).

**Exit criteria:** a merchant can run daily operations without touching the
database or asking us for help.

*Status: **done** (2026-07-15) — scheduled event drains, fulfillment with
tracking, refunds through the payment port (stock restored by event),
sales analytics, Stripe webhook reconciliation, and buyer email via Resend
(receipt + shipping). Follow-up: verify a sending domain in Resend to email
arbitrary buyers.*

## Phase 4 — Platform expansion

Each of these is its own project, gated on the core being solid:

- **Point of sale** — offline-tolerant client syncing to the same inventory
  and orders services. Depends on inventory reservations being rock-solid.
- **Theme system / visual builder** — the drag-and-drop editor. Storefront
  themes stay data-driven from day one so this is an editor project, not a
  storefront rewrite.
- **Public API & app ecosystem** — versioned public API, OAuth apps,
  outbound webhooks for third parties.
- **In-house payments** — only with real volume and a compliance budget.

*Status (2026-07-16): POS (ADR-010), public API v1 with keys, and outbound
webhooks are live; the storefront ships data-driven themes (ADR-009) as the
builder's foundation. Remaining Phase 4 ambitions — the visual theme
builder, dedicated POS hardware, OAuth apps — await real merchant demand.*

## Cross-cutting workstreams (every phase)

- **Observability** — structured logs (`pkg/logger`), metrics, tracing;
  added per-service as each service is born, not retrofitted.
- **Security** — secret management, authz checks at handler boundaries,
  dependency scanning in CI.
- **Data** — backups and migration discipline from the first production
  deploy.

---

## Working process

- One GitHub **milestone per phase**; issues are vertical slices
  ("merchant can add a product with variants"), not layers ("build catalog
  repository").
- Every decision marked **(ADR)** above gets a numbered ADR in `docs/adrs/`
  *before* the code that depends on it.
- `services/example` is deleted once the first real service (merchants)
  proves the template end-to-end.
