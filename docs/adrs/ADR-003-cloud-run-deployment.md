# ADR-003: Deploy services on Cloud Run

**Status**: Accepted
**Date**: 2026-07-15

## Context

The roadmap's Phase 0 requires picking a deployment target before the first
real service exists, so every later service inherits a working pipeline
instead of retrofitting one. The platform is a set of stateless Go HTTP
services (Gin) backed by Postgres — no in-process state, no sticky sessions.

The dominant constraints are cost and operations: the project is bootstrapped
by a solo founder. Pre-launch and early-launch traffic is near zero, so
anything always-on is money spent on idle capacity, and anything that needs
tending (clusters, nodes, patching) is founder time spent off the product.

One piece of the architecture pushes back: the outbox relay (ADR-002)
currently runs as a background goroutine inside each service. Serverless
platforms throttle CPU outside request handling and scale to zero, so a
polling goroutine cannot be relied upon there.

## Decision

We will deploy all services as Cloud Run services, billed per request with
scale-to-zero. The outbox relay's drain loop moves off the background
goroutine: each service exposes an internal drain endpoint that calls
`Relay.DrainOnce`, invoked by Cloud Scheduler (and still callable manually
and in tests).

## Alternatives considered

- **Managed Kubernetes (GKE/EKS)** — maximum control, but a cluster is
  always-on money and standing operational work (upgrades, node pools,
  ingress). Buying flexibility we have no workload for yet, priced for a
  team we don't have.
- **ECS on Fargate** — closest AWS equivalent, but no scale-to-zero for
  services behind a load balancer; the ALB alone is a fixed monthly cost
  above our entire expected Cloud Run bill.
- **Fly.io** — genuinely cheap and simple, with scale-to-zero machines. A
  credible runner-up; Cloud Run wins on the surrounding managed ecosystem
  (Cloud Scheduler for the relay, Secret Manager, IAM, Artifact Registry)
  and on being the less bus-factor-risky vendor for a commerce platform.
- **A VPS with docker compose** — cheapest sticker price and fine for a
  demo, but single point of failure, manual TLS/deploys/patching, and no
  scaling path; founder time is the scarcest resource we have.

## Consequences

Easier: near-zero cost at near-zero traffic; deploys are a container push;
TLS, autoscaling, logging, and revisions/rollbacks come managed; the
CLAUDE.md service shape (stateless, env-configured) is already Cloud
Run-compatible.

Harder: cold starts add latency after idle (acceptable pre-launch; buy
`min-instances=1` per user-facing service only when traffic justifies it);
nothing may depend on always-running background goroutines — the relay is
the first casualty and any future worker must be a Cloud Run job or
scheduler-triggered endpoint from day one; request timeouts cap long-running
work.

The database is deliberately **not** decided here. Cloud SQL's smallest
sensible instance would dwarf the compute bill at this stage; serverless
Postgres (e.g. Neon, Supabase) fits the bootstrap budget better but leaves
GCP's network. That trade-off gets its own ADR when the first service ships.

Follow-up work:

- Convert the relay wiring in `services/example/cmd/main.go` from a
  background goroutine to an authenticated internal drain endpoint calling
  `DrainOnce`; document the Cloud Scheduler pairing.
- Add a Dockerfile to the service template honoring `$PORT`.
- Set up one GCP project with Artifact Registry and a deploy workflow (CI
  builds, pushes, `gcloud run deploy`) for the first real service.

Revisit triggers: sustained traffic making per-request billing more
expensive than provisioned capacity; a workload that genuinely needs
long-lived processes (e.g. websockets at scale, a broker consumer) — at
which point the escape hatch is the container itself, which runs anywhere.
