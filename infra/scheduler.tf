# --- Outbox drains and sweeps (ADR-003) --------------------------------------
# Cloud Run throttles CPU outside requests, so the outbox relay cannot run as
# a background goroutine; Cloud Scheduler POSTs the internal drain endpoints
# instead, authenticated with a bearer token from Secret Manager.
#
# These jobs were originally created by hand and deleted in Aug 2026 when the
# project was suspended: firing every 10 minutes kept the Neon compute awake
# ~60% of the time (free tier autosuspends only after 5 idle minutes) and
# burned the entire monthly compute allowance with zero traffic. They are
# codified here paused so `terraform apply` restores them without losing the
# configuration. When resuming: unpause, and reconsider the cadence — every
# 10 minutes costs ~100 CU-hrs/month on Neon; opportunistic post-write drains
# with a slow scheduled safety net would cost almost nothing.

locals {
  # service => internal endpoint path invoked by its scheduler job
  drain_jobs = {
    catalog   = { job = "catalog-outbox-drain", path = "/internal/outbox/drain" }
    orders    = { job = "orders-outbox-drain", path = "/internal/outbox/drain" }
    inventory = { job = "inventory-reservations-sweep", path = "/internal/reservations/sweep" }
  }
}

# Bearer tokens presented by Cloud Scheduler and mounted into each service as
# OUTBOX_DRAIN_TOKEN (deploy workflow reads <service>-drain-token:latest).
resource "random_password" "drain_token" {
  for_each = local.drain_jobs

  length  = 48
  special = false
}

resource "google_secret_manager_secret" "drain_token" {
  for_each = local.drain_jobs

  secret_id = "${each.key}-drain-token"

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "drain_token" {
  for_each = local.drain_jobs

  secret      = google_secret_manager_secret.drain_token[each.key].id
  secret_data = random_password.drain_token[each.key].result
}

resource "google_cloud_scheduler_job" "drain" {
  for_each = local.drain_jobs

  name      = each.value.job
  region    = var.region
  schedule  = "*/10 * * * *"
  time_zone = "Etc/UTC"

  # Project suspended (Aug 2026) — see header comment before unpausing.
  paused = true

  http_target {
    http_method = "POST"
    # Cloud Run URLs are deterministic per project+region (same suffix as the
    # Pub/Sub push endpoints in events.tf).
    uri = "https://${each.key}-bjm36sbwlq-uc.a.run.app${each.value.path}"

    headers = {
      "Authorization" = "Bearer ${random_password.drain_token[each.key].result}"
    }
  }

  depends_on = [google_project_service.apis]
}
