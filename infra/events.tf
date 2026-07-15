# --- Event backbone transport (ADR-002 amendment) ---------------------------
# The catalog relay publishes outbox envelopes to this topic; consumers get
# push subscriptions (Cloud Run scale-to-zero forbids pull subscribers,
# ADR-003). Pushes are authenticated with Google-signed OIDC tokens from the
# pusher service account — no shared secrets.

resource "google_pubsub_topic" "catalog_events" {
  name = "catalog-events"

  depends_on = [google_project_service.apis]
}

# The catalog service (runtime SA) publishes.
resource "google_pubsub_topic_iam_member" "catalog_publisher" {
  topic  = google_pubsub_topic.catalog_events.name
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:${google_service_account.run_services.email}"
}

# Identity presented on push deliveries; consumers verify this exact SA.
resource "google_service_account" "pubsub_pusher" {
  account_id   = "pubsub-pusher"
  display_name = "Pub/Sub push deliveries"
}

# The Pub/Sub service agent mints OIDC tokens for the pusher SA.
data "google_project" "project" {}

resource "google_service_account_iam_member" "pubsub_agent_token_creator" {
  service_account_id = google_service_account.pubsub_pusher.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:service-${data.google_project.project.number}@gcp-sa-pubsub.iam.gserviceaccount.com"
}

variable "inventory_push_endpoint" {
  description = "Inventory service Pub/Sub push URL (Cloud Run URLs are deterministic per project+region)"
  type        = string
  default     = "https://inventory-bjm36sbwlq-uc.a.run.app/internal/events/pubsub"
}

resource "google_pubsub_subscription" "catalog_events_inventory" {
  name  = "catalog-events-inventory"
  topic = google_pubsub_topic.catalog_events.id

  ack_deadline_seconds = 30

  push_config {
    push_endpoint = var.inventory_push_endpoint

    oidc_token {
      service_account_email = google_service_account.pubsub_pusher.email
      audience              = var.inventory_push_endpoint
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  # Undeliverable messages stop retrying after a week; revisit with a dead
  # letter topic when there is real traffic to lose.
  expiration_policy {
    ttl = "" # never expire the subscription itself
  }
  message_retention_duration = "604800s"

  depends_on = [google_service_account_iam_member.pubsub_agent_token_creator]
}

# --- orders-events: paid orders drive stock (issue #18) ----------------------

resource "google_pubsub_topic" "orders_events" {
  name = "orders-events"

  depends_on = [google_project_service.apis]
}

resource "google_pubsub_topic_iam_member" "orders_publisher" {
  topic  = google_pubsub_topic.orders_events.name
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:${google_service_account.run_services.email}"
}

resource "google_pubsub_subscription" "orders_events_inventory" {
  name  = "orders-events-inventory"
  topic = google_pubsub_topic.orders_events.id

  ack_deadline_seconds = 30

  push_config {
    push_endpoint = var.inventory_push_endpoint

    oidc_token {
      service_account_email = google_service_account.pubsub_pusher.email
      audience              = var.inventory_push_endpoint
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  expiration_policy {
    ttl = ""
  }
  message_retention_duration = "604800s"

  depends_on = [google_service_account_iam_member.pubsub_agent_token_creator]
}
