# ADR-004: Terraform for infrastructure provisioning

**Status**: Accepted
**Date**: 2026-07-15

## Context

ADR-003 committed us to Cloud Run on Google Cloud. The surrounding
infrastructure — enabled APIs, Artifact Registry, service accounts, IAM
bindings, Workload Identity Federation, later Cloud Scheduler jobs and
Pub/Sub topics — has to be created somehow, and a solo founder clicking
through the console produces infrastructure nobody can reproduce, audit, or
recover. At the same time, the operation is one person on a bootstrap
budget: heavyweight IaC platforms and multi-environment module hierarchies
are cost without benefit here.

## Decision

We will manage GCP infrastructure with Terraform: a single root module in
`infra/`, state in a versioned GCS bucket (`cloud-commerce-prd-tfstate`),
applied manually from the founder's machine. One GCP project
(`cloud-commerce-prd`) for now.

## Alternatives considered

- **Console click-ops / ad-hoc gcloud** — fastest today, unreproducible
  forever. The first "how was this configured?" incident costs more than
  Terraform's entire learning curve.
- **Pulumi** — IaC in Go would match the codebase, but the provider
  ecosystem, documentation mass, and AI/community answer density for GCP
  are all thinner than Terraform's; hiring familiarity later also favors
  Terraform.
- **Terraform Cloud / Atlantis / CI-applied plans** — remote state locking,
  plan review, drift detection: team problems. One operator applying from
  one machine doesn't have them yet. Revisit with the first additional
  operator.
- **Config Connector / KCC** — requires a GKE cluster, which ADR-003 just
  declined to run.

## Consequences

Easier: the entire platform is described in `infra/`, reviewable in PRs and
rebuildable from scratch; deleted-by-accident is recoverable; the WIF setup
(keyless GitHub Actions auth) is exactly the kind of fiddly IAM that
benefits from being written down as code.

Harder: Terraform state is now a thing to care about (mitigated: versioned
GCS bucket); manual `terraform apply` means infra changes are not gated by
CI (acceptable at one operator); provider upgrades occasionally demand
attention.

Conventions:

- One root module; extract child modules only when a real second consumer
  of the pattern exists (same rule as service extraction).
- No secrets in state where avoidable — secret *values* go into Secret
  Manager out-of-band (`gcloud secrets versions add`); Terraform manages
  containers and IAM, not payloads.
- `terraform plan` before every apply; the lock file is committed.

Revisit triggers: a second operator (add CI-applied plans and state
locking discipline), or a second environment (introduce workspaces or a
directory per environment — decide then, not now).
