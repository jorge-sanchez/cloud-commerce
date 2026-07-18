output "workload_identity_provider" {
  description = "Full WIF provider name for google-github-actions/auth"
  value       = google_iam_workload_identity_pool_provider.github.name
}

output "deployer_service_account" {
  description = "Service account GitHub Actions impersonates"
  value       = google_service_account.github_deployer.email
}

output "runtime_service_account" {
  description = "Service account Cloud Run services run as"
  value       = google_service_account.run_services.email
}

output "artifact_registry" {
  description = "Docker repository base path for image tags"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.services.repository_id}"
}

output "media_bucket" {
  description = "Product media bucket (catalog MEDIA_BUCKET)"
  value       = google_storage_bucket.media.name
}
