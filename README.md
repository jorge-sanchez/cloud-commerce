# Cloud Commerce

Cloud Commerce (working name) is a cloud-based, Software-as-a-Service commerce platform. The goal: give entrepreneurs and businesses everything they need to build, manage, and operate an online store and a physical retail presence from a single platform.

## Vision

The platform will cover the complete commerce stack for business owners:

- **Online storefronts** — build a website from drag-and-drop themes, no coding skills required.
- **Payment processing** — accept credit cards, digital wallets, and local payment methods through a built-in payments service.
- **Point of sale** — run physical checkout on the same platform, with in-person and online inventory synced in real time.
- **Inventory and orders** — one central hub to track stock levels, fulfill orders, print shipping labels, and follow analytics.

## Status

Early days. The current codebase is the platform foundation: a Go multi-module monorepo with shared infrastructure packages, a reference service demonstrating every architectural convention, and CI enforcing them. Product services (catalog, orders, payments, storefront, POS) will be built on top of this skeleton.

## Architecture

The repository is a multi-module monorepo managed with [`go.work`](go.work). Each service owns its `go.mod`; shared infrastructure lives in `pkg/` (one module per package).

```
services/<name>/          # one module per service
├── cmd/main.go
├── migrations/
└── internal/
    ├── domain/           # entities, value objects, repository interfaces, domain events
    ├── service/          # application services (orchestration only)
    ├── repository/       # persistence adapters (PostgreSQL, Redis)
    ├── handler/          # HTTP handlers (Gin)
    └── producer/         # event-publishing port adapters

pkg/                      # shared modules: errors, logger, pagination, testdb
docs/adrs/                # architecture decision records
```

Services follow domain-driven design: business rules live on domain entities, repositories only load and save, and `internal/` packages are never shared across service boundaries. `services/example` is a working reference implementation of every rule — read it before writing a new service.

Conventions are documented in [CLAUDE.md](CLAUDE.md) and significant decisions are recorded as ADRs in [docs/adrs/](docs/adrs/).

## Getting started

Prerequisites: Go 1.25+, Docker, and [golang-migrate](https://github.com/golang-migrate/migrate) for database migrations.

```sh
make setup            # sync the go workspace and install git hooks
make dev-infra        # start PostgreSQL via docker compose
make migrate          # apply migrations
make run-example      # run the reference service
```

Run `make help` to see every available target.

## Development

```sh
make test             # unit tests across all modules
make test-integration # integration tests (testcontainers locally, shared DB in CI)
make lint             # golangci-lint per module, mirrors CI
make build            # build all modules
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) (`type(scope): subject`), and pull requests must reference the issue they close (`Closes #N`). CI runs lint, tests, and an API-contract ratchet check on every PR.
