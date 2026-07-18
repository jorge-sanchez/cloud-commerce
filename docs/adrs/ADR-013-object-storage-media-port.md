# ADR-013: Product media through an object-storage port, GCS as the provider

**Status**: Accepted
**Date**: 2026-07-17

## Context

RFC-003 (accepted) adds product images. The load-bearing decision is where
image bytes live and how they get there. The catalog service runs scale-to-zero
on Cloud Run (ADR-003); routing multi-megabyte uploads through it means memory
pressure, request timeouts, and double egress. Storing bytes in Neon
(ADR-005) would detonate the free tier. And the same seam will later serve store
logos and hero imagery (Tier-2), so it should not be product-specific.

This is the canonical integrate-don't-build capability (roadmap principle 2):
object storage is commodity, and the account/keys should not leak into the
domain.

## Decision

Product media goes through a **`MediaStore` port** owned by the catalog
service: `SignUpload` mints a short-lived V4 signed `PUT` URL, `Promote` moves a
finalized upload from its staging key to its permanent key (returning the
authoritative content type and size), and `Delete` removes an object. The
buyer-facing bytes never flow through the service — the browser uploads directly
to the signed URL and reads objects directly from the bucket.

**GCS is the launch provider.** V4 signing uses the runtime service account's
IAM `SignBlob` (`roles/iam.serviceAccountTokenCreator` on itself), so there is
no key file and nothing new in Secret Manager. Objects are public-read under
content-addressed keys with an immutable `Cache-Control`, served directly from
`storage.googleapis.com`. Uploads land under a `staging/` prefix and are
promoted only on finalize, so a storage lifecycle rule can reap un-finalized
orphans without touching live images.

The read API composes public URLs from a configured base
(`MEDIA_PUBLIC_BASE_URL`), not from stored data — so the serving host can move
(direct GCS today, a CDN behind the custom-domain load balancer later) without
a migration.

## Alternatives considered

- **Proxy uploads through the service** — self-contained, no CORS, but puts
  buyer-facing megabytes on a scale-to-zero container: memory spikes, timeouts,
  double egress. Signed direct-to-bucket is the standard answer.
- **Bytes in Postgres (`bytea`)** — transactional with the product, but blows
  the Neon free tier and streams blobs through the DB and the app.
- **A dedicated media/asset service** — no independent scale or lifecycle;
  images belong to the Product aggregate (fewer-coarse-services, principle 3).
- **Third-party DAM (Cloudinary/imgix)** — upload+transform+CDN in one, but
  per-image pricing and lock-in for a capability GCS covers at launch; the port
  keeps it swappable.
- **Signed key with no staging prefix** — simpler keys, but then a lifecycle
  rule cannot distinguish orphaned uploads from live images. Staging + promote
  keeps cleanup safe.

## Consequences

Easier: uploads scale independently of the service; no signing key to manage;
swapping providers or fronting a CDN is a config change, not a data migration;
the same port serves Tier-2 store imagery.

Harder: bucket CORS must list the admin origin; V4 signing needs the
`serviceAccountTokenCreator` self-grant; finalize is a two-step (promote then
record) whose failure modes need best-effort object cleanup; derivative
generation (thumbnails/WebP) is deferred and will hang off the
`catalog.product_media_updated` event when demand arrives.

Revisit triggers: a CDN/custom domain (Tier-2 #8) — front the bucket and move
the base URL; image transforms becoming necessary — add a processor on the
media event; a second storage provider — implement the port, no domain change.
