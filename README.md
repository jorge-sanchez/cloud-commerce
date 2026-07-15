# go-service-template

A template for Go backend monorepos: multi-module `go.work` layout, DDD
service structure, shared infrastructure packages, CI with a docs-only
fast path, and the conventions that make it all enforceable.

## What's inside

| Piece | Purpose |
|---|---|
| `CLAUDE.md` | The house rules: DDD layering, options pattern, error handling, test budget, wire contracts. Read this first. |
| `pkg/errors` | `AppError` with sentinel errors, `errors.Is`/`As` support, and Gin response helpers. |
| `pkg/pagination` | Query-param parsing + the blessed `{items, total, page, page_size}` envelope. |
| `pkg/logger` | Zap logger with trace-ID extraction, request-scoped context injection, and a PII scrub core. |
| `pkg/testdb` | Per-test-scope Postgres provisioning: shared server in CI (`TEST_POSTGRES_DSN`), testcontainer locally. |
| `services/example` | A reference service exercising every convention: entity-owned transitions, intent-named repository, options-pattern service, `apitypes.go` wire contract, hand-rolled fakes, integration tests, migrations. |
| `.github/workflows` | Test + Lint with a `changes` gate job so docs-only PRs stay green even with required checks. |
| `scripts/` | Pre-commit hook installer (golangci-lint per module) and the `gin.H` success-body ratchet. |

## Using the template

1. Create your repo from this template (`gh repo create my-service --template jorge-sanchez/go-service-template`).
2. Rename the module root — find-and-replace `github.com/jorge-sanchez/go-service-template` with your module path across `go.work`, `go.mod` files, Go imports, and `.golangci.yml` (`local-prefixes`).
3. `make setup` — syncs the workspace and installs the pre-commit hook.
4. `make dev-infra && make migrate && make run-example` — Postgres up, schema applied, service on :8080.
5. Build your first real service by copying the `services/example` layout; delete `services/example` when it has served its purpose.

## Day-to-day

```
make test               # unit tests, all modules
make test-integration   # integration tests (Docker required locally)
make lint               # golangci-lint per module, GOWORK=off (mirrors CI)
make ratchet            # no new gin.H success bodies
make migrate-create NAME=create_foo
```

## Requirements

- Go ≥ 1.25
- Docker (local Postgres + testcontainers)
- [golangci-lint v2](https://golangci-lint.run/), [golang-migrate](https://github.com/golang-migrate/migrate)
