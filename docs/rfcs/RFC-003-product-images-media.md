# RFC-003: Product images & media

**Status**: Accepted — implemented across PRs #68–#70 (2026-07-19);
live verification pending infra apply + catalog redeploy
**Author(s)**: Claude (with Jorge Sanchez)
**Date**: 2026-07-17
**Related**: Reviewed as Discussion #64 (full design text there); Discussion
#47 (Tier 1, item 3); ADR-007 (admin SPA); ADR-009 (storefront — deferred
image handling); spawns ADR-013 (object-storage media port). This file is the
canonical record of the accepted design.

## Summary

Product images end to end: browsers upload directly to Google Cloud Storage
via short-lived signed URLs (bytes never touch Cloud Run), image metadata is
persisted as a child collection of the Product aggregate in catalog, and
public image URLs render in the storefront and admin. The last thing making
the storefront look unfinished, and the third leg of Tier-1 checkout work.

## Design (accepted)

1. **Bytes never touch the service.** Admin requests a V4 signed `PUT` URL
   (`POST /v1/products/:id/images:sign`), the browser uploads straight to a
   `staging/` key, then finalize (`POST .../images`) promotes the object to
   its permanent content-addressed key and records the row. Catalog signs as
   the runtime SA via IAM `SignBlob` — **no key file** (ADR-013).
2. **Images are a Product child collection** (CLAUDE.md aggregate rule):
   `AttachImage`/`ReorderImages`/`RemoveImage`/`SetImageAlt` are entity
   transitions (caps: 10/product, 5 MiB, 4096 px) persisted atomically with a
   `catalog.product_media_updated` outbox event. Table `product_images`,
   tenant-led index (ADR-001), nullable `variant_id` reserved.
3. **`MediaStore` port (ADR-013)**: GCS adapter at launch, S3/R2/Cloudinary
   swappable; the same port serves Tier-2 store logo/hero imagery.
4. **Serving**: public-read, content-addressed keys, immutable `Cache-Control`,
   served directly from GCS. Public URLs are composed at read time from a
   configured base (`MEDIA_PUBLIC_BASE_URL`), so the serving host can move to a
   CDN behind the custom-domain load balancer (Tier-2) without a data
   migration. Storefront lazy-loads with intrinsic dimensions; empty galleries
   render a placeholder (no backfill).

## Resolutions (review round, Discussion #64)

- **Caps**: 10 images/product, 5 MiB, 4096 px.
- **Orphan cleanup**: uploads land under `staging/`; a 1-day lifecycle rule
  reaps un-finalized objects there without touching live images (a blanket
  age rule would have deleted real photos — the staging split resolves it).
- **Draft-product images public-read** (unguessable UUID keys).
- **Alt text editable after upload** (`PUT .../images/:imageId`), added with
  the admin UI — accessibility alt is not useful write-once.

## Out of scope (recorded)

Derivative generation (thumbnails, WebP/AVIF, EXIF-orientation) — deferred
behind the `product_media_updated` event; video/rich media (the
`content_type` column generalizes); per-variant image switching (nullable
`variant_id` reserved); store logo/hero (Tier-2 #9, same port); primary image
on order/receipt/emails; custom-domain CDN fronting (Tier-2 #8); the
product-editing feature this unblocks (#61).

## Rollout

Backbone (#65 → PR #68: catalog + infra + ADR-013) → admin upload UI
(#66 → PR #69) → storefront rendering (#67 → PR #70). Post-deploy: apply the
media-bucket Terraform and redeploy catalog with `MEDIA_BUCKET`/
`MEDIA_SIGNER_SA`, then live-verify both directions (upload appears on the
storefront served from GCS with the immutable cache header; delete drops it
from both). Existing products predate images — galleries render a placeholder,
no backfill.
