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

# Fetch application message limits and monthly quota usage.
data "pushover_limits" "app" {}

output "monthly_quota" {
  description = "Total monthly message allowance."
  value       = data.pushover_limits.app.limit
}

output "messages_remaining" {
  description = "Messages remaining in current billing period."
  value       = data.pushover_limits.app.remaining
}

output "quota_reset_timestamp" {
  description = "Unix timestamp when the quota resets."
  value       = data.pushover_limits.app.reset
}
