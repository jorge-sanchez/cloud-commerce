# ADR-008: Payments through a provider port, Stripe as the provider

**Status**: Accepted
**Date**: 2026-07-15

## Context

Phase 2 ends with a paid order. Handling card data ourselves would put the
platform in PCI scope — ADR-003's reasoning applies doubly here: compliance
is capital and founder-time we do not have, and payment processing is the
canonical integrate-don't-build capability (roadmap principle 2).

The orders service already owns the order state machine; `MarkPaidIfPayable`
and the `orders.order_paid` outbox event exist. What is missing is the seam
to a provider, and a way to exercise the full flow before a provider
account exists.

## Decision

Payments go through a **`PaymentGateway` port** owned by the orders
service: `CreatePayment` initializes payment for a pending order and
returns a provider handoff (reference + client secret); `ConfirmPayment`
verifies the completed payment before the order is marked paid. The
buyer-facing flow is `POST /v1/public/orders/:id/pay` then
`POST /v1/public/orders/:id/pay/confirm` — shaped like real provider flows
so the adapter swap changes no API.

The **first implementation is a deliberate fake** (`gateway.FakeGateway`):
it issues deterministic references and confirms only matching ones. It
ships the entire flow — state machine, events, stock decrement — while the
Stripe account is created. **Stripe** is the chosen provider (#19):
dominant documentation and tooling, fine bootstrap pricing, and test mode
for free end-to-end verification.

## Alternatives considered

- **Stripe SDK calls inline in the service** — couples the domain flow to
  one provider and makes the pre-account period unshippable. The port
  costs one interface.
- **A payments microservice** — a second deployable for what is today two
  methods; the port can be extracted into a service later without changing
  the domain (same rule as service extraction everywhere else).
- **Other providers (Adyen, MercadoPago, PayPal)** — MercadoPago matters
  for LATAM buyers and can become a second adapter behind the same port;
  none beat Stripe as the first integration.

## Consequences

Easier: the whole first-dollar flow is testable today; the Stripe adapter
(#19) is an implementation of two methods plus a webhook route; a second
regional provider is additive.

Harder: the fake gateway must never run against real buyers — it is wired
only while `PAYMENT_PROVIDER=fake`, and #19 replaces the default; webhook
reconciliation (provider → confirm) arrives with the real adapter, so
until then confirmation is buyer-initiated.

Revisit triggers: the Stripe account existing (#19); LATAM conversion
data suggesting MercadoPago; refunds (Phase 3) extend the port.
