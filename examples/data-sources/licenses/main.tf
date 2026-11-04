terraform {
  required_providers {
    pushover = {
      source  = "Josh-Archer/pushover"
      version = "~> 1.0"
    }
  }
}

provider "pushover" {
  api_token = var.pushover_api_token
}

variable "pushover_api_token" {
  type      = string
  sensitive = true
}

# Fetch remaining pre-paid license credits for the application (read-only).
data "pushover_licenses" "app" {}

output "license_credits_remaining" {
  description = "Unassigned pre-paid license credits remaining on the application."
  value       = data.pushover_licenses.app.credits
}
