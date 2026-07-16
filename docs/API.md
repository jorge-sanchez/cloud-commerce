# Cloud Commerce Public API v1

Third-party integrations authenticate with an **API key** (created by the
store owner in the admin or via `POST /v1/api-keys`), exchanged for a
short-lived bearer token:

```
POST https://merchants-bjm36sbwlq-uc.a.run.app/v1/auth/token
{"api_key": "cck_..."}          → {"token": "<JWT, ~1h, role=api>"}
```

Use the token as `Authorization: Bearer <token>` against any service.
Tokens carry the `api` role: full data access, no staff or settings
management. Re-exchange when a request returns 401.

| Surface | Base URL |
|---|---|
| Merchants (store, staff, keys) | https://merchants-bjm36sbwlq-uc.a.run.app |
| Catalog (products, collections) | https://catalog-bjm36sbwlq-uc.a.run.app |
| Inventory (stock, locations) | https://inventory-bjm36sbwlq-uc.a.run.app |
| Orders (orders, POS, analytics) | https://orders-bjm36sbwlq-uc.a.run.app |

Wire shapes are owned by each service's `internal/handler/apitypes.go`
(TypeScript mirrors in `services/admin/web/src/types/`). **Stability**:
v1 paths are additive-only — fields may be added, never removed or
renamed; breaking changes get a new version prefix. Errors are uniform
`{code, message}`. Lists use `{items, total, page, page_size}` (offset)
envelopes. Outbound webhooks are tracked in issue #44.
