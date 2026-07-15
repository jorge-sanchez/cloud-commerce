# CLAUDE.md — Cloud Commerce

Coding conventions and architectural rules for this codebase. Follow these exactly.

---

## Repository layout

Multi-module monorepo managed with `go.work`. Each service under `services/` has its own `go.mod`. Shared infrastructure lives in `pkg/` (one module per package).

```
services/<name>/
├── cmd/main.go
├── go.mod
├── migrations/
└── internal/
    ├── domain/       # entities, value objects, repository interfaces, domain events
    ├── service/      # application services (orchestration only)
    ├── repository/   # persistence adapters (PostgreSQL, Redis)
    ├── handler/      # HTTP handlers (Gin)
    └── producer/     # event-publishing port adapters
```

`internal/` packages are truly internal — never import across service boundaries. Cross-service contracts go through shared `pkg/` packages or an explicit RPC layer.

`services/example` is a reference implementation of every rule below. Read it before writing a new service; delete it once you have real services.

---

## DDD rules

### Domain entities own their logic

Business rules live on the entity, not in the service or repository.

```go
// Good — entity decides its own transitions
func (w *Widget) Publish() error {
    if !w.CanPublish() {
        return fmt.Errorf("%w: status is %q", ErrNotPublishable, w.Status)
    }
    w.Status = WidgetStatusPublished
    return nil
}

// Bad — service or repository checking status directly
if widget.Status == "draft" {
    widget.Status = "published"
}
```

### Repositories persist what entities decide

The repository loads the entity, calls the domain method, then saves the result. No business logic in the persistence layer.

```go
// PublishIfPublishable in the repository
widget := row.toDomain()
if err := widget.Publish(); err != nil {   // entity decides
    return nil, apperrors.ErrConflict.Wrap(err)
}
_, err = tx.ExecContext(ctx, `UPDATE ... SET status = $1`, widget.Status, ...)
```

### Aggregate roots own their collections atomically

When an aggregate root owns a collection, persist both together in a single transaction. Never expose a separate repository method that lets callers insert child records independently.

```go
// Good — single atomic operation on the aggregate
SaveWithVariants(ctx context.Context, tenantID string, job *Job) (*Job, error)

// Bad — caller does two separate inserts with no rollback between them
Insert(ctx, tenantID, job)
variantRepo.InsertBatch(ctx, job.ID, variants)
```

### Repository interface method names express intent

Use domain-specific names, not generic CRUD. Interfaces live in `domain/`, implementations in `repository/`.

```go
// Good
PublishIfPublishable(ctx, tenantID, id string) (*Widget, error)
SaveNew(ctx context.Context, tenantID string, w *Widget) (*Widget, error)
ListByTenant(ctx, tenantID string, page, pageSize int) ([]*Widget, int, error)

// Bad
Update(ctx, w *Widget) (*Widget, error)
Create(ctx, w *Widget) (*Widget, error)
```

---

## Constructors — options pattern

Required dependencies are positional. Optional dependencies use `Option` functions. Never add a new constructor variant (`NewXxxWithY`); add a `WithY` option instead. Applies to services, repositories, and infrastructure components alike.

```go
type Option func(*PostgresWidgetRepository)

func WithEventRecorder(rec EventRecorder) Option {
    return func(r *PostgresWidgetRepository) { r.events = rec }
}

func NewPostgresWidgetRepository(db *sql.DB, opts ...Option) *PostgresWidgetRepository {
    r := &PostgresWidgetRepository{db: db}
    for _, opt := range opts { opt(r) }
    return r
}
```

Mark optional fields with a comment:

```go
type PostgresWidgetRepository struct {
    db     *sql.DB       // required
    events EventRecorder // may be nil
}
```

---

## Error handling

Use the sentinel errors from `pkg/errors`. Never return raw `errors.New` or `fmt.Errorf` from service or repository methods.

```go
// Return a sentinel directly when nothing to wrap
return nil, apperrors.ErrNotFound

// Wrap when there is an underlying cause to preserve
return nil, apperrors.ErrInternal.Wrap(err)

// Domain sentinel errors for entity-level failures
var ErrNotPublishable = errors.New("widget cannot be published in its current status")
// then in method: return fmt.Errorf("%w: status is %q", ErrNotPublishable, w.Status)
```

Test error assertions with `ErrorIs`/`ErrorAs`, never string-match:

```go
var appErr *apperrors.AppError
require.ErrorAs(t, err, &appErr)
assert.Equal(t, "VALIDATION_ERROR", appErr.Code)
```

---

## API wire contracts

### Success bodies are named structs — never `gin.H`

Every 200/201 body comes from an exported, JSON-tagged struct in the service's `internal/handler/apitypes.go`. `gin.H` stays fine for error bodies, which are the uniform `{code, message}` shape from `pkg/errors`.

```go
// Good — apitypes.go owns the contract
c.JSON(http.StatusOK, ListWidgetsResponse{Items: items, Total: total, Page: page, PageSize: pageSize})

// Bad — anonymous map: un-generatable, un-greppable, drifts silently
c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
```

Enforced by `scripts/check_ginh_success_ratchet.sh` (CI: "API Types Drift" job): per-module baseline counts may only go down.

Named wire structs also let you generate TypeScript types (e.g. with tygo) if you add a frontend — never hand-write a response shape the backend already owns.

### Blessed list envelopes

Two envelopes, one per pagination style — never invent a third:

| Style | Envelope | Source |
|---|---|---|
| Offset | `{items, total, page, page_size}` | `pkg/pagination.Response` or a typed local equivalent |
| Cursor | `{items, next_cursor}` | keyset pagination; `next_cursor` empty when the page is not full |

---

## Tests

### Test budget

Every test file declares its budget at the top. Each behavior gets at most two tests (happy path + error case).

```go
// Test Budget: 4 distinct behaviors × 2 = 8 max unit tests
// Actual: 6
//
// Behavior 1: Create — persists a draft widget; empty name is a validation error
// Behavior 2: Get — delegates to repo and passes errors through
// ...
```

### Unit vs integration

| File suffix / build tag | What it tests |
|---|---|
| `_test.go` (no tag) | Unit — fakes at port boundaries, no I/O |
| `_sqlmock_test.go` (no tag) | Repository layer — SQL mock for error paths |
| `//go:build integration` | Integration — real Postgres via `pkg/testdb` |

Integration tests provision databases through `pkg/testdb.Open(t)` — a shared server when `TEST_POSTGRES_DSN` is set (CI), a testcontainer otherwise. Never start raw Postgres testcontainers yourself.

### Cross-tenant negative cases (ADR-001)

The `tenant_id` filter in repositories is the tenancy security boundary. Every repository's integration suite must include at least one cross-tenant negative case: read another tenant's row and expect `apperrors.ErrNotFound` (another tenant's data must be indistinguishable from missing data).

### Test doubles — hand-rolled fakes only

No `gomock`, no `testify/mock`. Write fakes at the port boundary (interfaces defined by the domain or service layer). Track call history in the fake struct for post-test assertions.

```go
type fakeDeliverer struct {
    err       error
    delivered []events.Envelope
}

func (f *fakeDeliverer) Deliver(_ context.Context, env events.Envelope) error {
    if f.err != nil {
        return f.err
    }
    f.delivered = append(f.delivered, env)
    return nil
}
```

Use compile-time interface checks in test files when it aids readability:

```go
var _ producer.Deliverer = (*fakeDeliverer)(nil)
```

### Naming

`Test<Type>_<Method>_<Condition>_<Expected>` — all segments in CamelCase, separated by underscores.

```
TestWidgetService_Create_EmptyName_ReturnsValidationErrorAndNoDBWrite
TestWidget_Publish_AlreadyPublished_ReturnsErrNotPublishable
```

Group tests with labeled comments matching the behavior number:

```go
// ---------------------------------------------------------------------------
// Behavior 3: Publish emits event built from the persisted widget
// ---------------------------------------------------------------------------
```

### Assertions

Use `require` when failure makes the rest of the test meaningless. Use `assert` otherwise. Always include a message on `Len` and other structural assertions:

```go
require.NoError(t, err)
require.Len(t, repo.saved, 1, "exactly one widget must be written")
assert.Equal(t, "widget-001", w.ID)
```

---

## Events

Domain events are defined in the domain package alongside the aggregate, wrapped in the shared envelope from `pkg/events`, and persisted through the transactional outbox (ADR-002) — never published directly from the request path.

The repository records the envelope in the **same transaction** as the state change it describes, via the `EventRecorder` port implemented by `producer.OutboxRecorder`:

```go
// inside PublishIfPublishable, before tx.Commit()
if r.events != nil {
    event := domain.NewWidgetPublishedEvent(widget, time.Now().UTC())
    env, err := events.New(widget.TenantID, widget.ID, domain.WidgetPublishedEventType, event.PublishedAt, event)
    if err != nil {
        return nil, apperrors.ErrInternal.Wrap(err)
    }
    if err := r.events.Record(ctx, tx, env); err != nil {
        return nil, err  // rolls back — state and event commit or fail together
    }
}
```

`producer.Relay` drains undelivered rows in insertion order and owns delivery retries — delivery failures never reach the request path. Delivery is at-least-once, so consumers must be idempotent (dedupe on envelope ID).

Build event payloads from the persisted domain object (what the entity decided), not from the raw request.

---

## Migrations

**Naming**: `NNNNNN_short_description.{up,down}.sql` — six-digit zero-padded sequence, per service under `services/<name>/migrations/`.

**Structure**: Lead with a comment explaining purpose. Use idempotent DDL (`IF NOT EXISTS` / `IF EXISTS`). Every `up` migration must have a matching `down` migration. When the database is empty and migrations run from scratch, edit the original migration file instead of creating a rename migration.

**Indexes**: Composite indexes on tenant-owned tables lead with `tenant_id` (ADR-001) — every query filters on it first. Platform infrastructure tables scanned across tenants (e.g. the outbox) are the exception.

---

## Git and PR conventions

**Commits**: Conventional Commits — `type(scope): subject`

```
feat(example): enforce Widget aggregate root boundary
fix(errors): preserve cause chain in Wrap
docs(adr): record decision to keep per-package modules
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`. Scope is the service or package name. Subject is imperative, lowercase, no trailing period.

**PR descriptions** must include `Closes #<issue>` (the issue closes automatically on merge — do not close it manually).

**Architecture decisions** are recorded as ADRs in `docs/adrs/` (see the template there).

---

## What not to do

- Do not add business logic to repository methods. Repositories load and save; entities decide.
- Do not create `NewXxxWithY` constructor variants. Add a `WithY` option instead.
- Do not use `gomock` or `testify/mock`. Write hand-rolled fakes.
- Do not string-match error messages in tests. Use `errors.Is`, `errors.As`, or check `AppError.Code`.
- Do not insert child records outside the aggregate's `SaveWith*` method.
- Do not publish events from the request path. Record them to the outbox inside the repository transaction (ADR-002); the relay owns delivery.
- Do not write a repository without a cross-tenant negative integration test (ADR-001).
- Do not serve 200/201 bodies as `gin.H` — named structs in `apitypes.go`; the ratchet check will fail the PR.
- Do not share `internal/` packages across service boundaries.
- Do not start raw Postgres testcontainers — use `pkg/testdb.Open(t)`.
- Do not close issues manually — include `Closes #N` in the PR description.
