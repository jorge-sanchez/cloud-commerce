# Terraform root module for cloud-commerce (ADR-004). One module, one
# project, GCS state — sized for a bootstrapped solo operation, not a
# platform team.

terraform {
  required_version = ">= 1.9"

  backend "gcs" {
    bucket = "cloud-commerce-prd-tfstate"
    prefix = "root"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}
