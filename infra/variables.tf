variable "project_id" {
  description = "GCP project ID"
  type        = string
  default     = "cloud-commerce-prd"
}

variable "region" {
  description = "Default region. us-central1 is Cloud Run tier-1 pricing."
  type        = string
  default     = "us-central1"
}

variable "github_repository" {
  description = "GitHub repository (owner/name) allowed to deploy via WIF"
  type        = string
  default     = "jorge-sanchez/cloud-commerce"
}
