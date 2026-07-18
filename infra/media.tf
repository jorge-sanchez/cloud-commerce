# --- Product media bucket (RFC-003, ADR-013) --------------------------------
# Buyer-visible product images. Browsers upload directly via V4 signed URLs
# (bytes never touch Cloud Run) and read objects straight from the bucket —
# public, immutable, content-addressed. A fronting CDN is deferred until the
# custom-domain load balancer exists (Tier-2); the base URL can move then
# without a data migration.

resource "google_storage_bucket" "media" {
  name     = "${var.project_id}-media"
  location = var.region

  # Uniform access: permissions live in IAM, not per-object ACLs.
  uniform_bucket_level_access = true

  # Browsers PUT directly to signed URLs from the admin origin.
  cors {
    origin          = var.media_cors_origins
    method          = ["PUT", "GET", "HEAD"]
    response_header = ["Content-Type"]
    max_age_seconds = 3600
  }

  # Un-finalized uploads live under staging/ and are reaped after a day.
  # Finalized images live under t/ and are never auto-deleted.
  lifecycle_rule {
    condition {
      age            = 1
      matches_prefix = ["staging/"]
    }
    action {
      type = "Delete"
    }
  }

  depends_on = [google_project_service.apis]
}

# Public read: product images are shown to anonymous buyers. Keys are
# unguessable UUIDs, so draft-product images stay effectively private.
resource "google_storage_bucket_iam_member" "media_public_read" {
  bucket = google_storage_bucket.media.name
  role   = "roles/storage.objectViewer"
  member = "allUsers"
}

# The catalog service (running as the runtime SA) creates, promotes, and
# deletes objects.
resource "google_storage_bucket_iam_member" "media_run_admin" {
  bucket = google_storage_bucket.media.name
  role   = "roles/storage.objectAdmin"
  member = "serviceAccount:${google_service_account.run_services.email}"
}

# V4 signing with no key file: the runtime SA signs upload URLs as itself via
# IAM SignBlob (iamcredentials API, already enabled).
resource "google_service_account_iam_member" "media_signer" {
  service_account_id = google_service_account.run_services.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.run_services.email}"
}
