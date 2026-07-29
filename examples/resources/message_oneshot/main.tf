# One-shot Pushover notify patterns.
#
# These examples show how to avoid accidental re-sends when unrelated plan
# churn would otherwise force replacement of pushover_message.

# --- Variables ---
variable "pushover_api_token" {
  description = "Pushover application API token."
  type        = string
  sensitive   = true
}

variable "pushover_user_key" {
  description = "Pushover user or group key to send notifications to."
  type        = string
  sensitive   = true
}

variable "app_version" {
  description = "Application version used as an idempotency key for versioned notifies."
  type        = string
  default     = "1.0.0"
}

variable "deployment_id" {
  description = "Opaque deployment identifier used with replace_triggered_by."
  type        = string
  default     = "deploy-001"
}

# --- Provider ---
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

# --- Pattern 1: True one-shot (never re-send after create) ---
# After the first apply, attribute edits in this block do not re-send.
resource "pushover_message" "bootstrap" {
  user_key = var.pushover_user_key
  message  = "Infrastructure bootstrap completed successfully."
  title    = "Bootstrap Complete"
  priority = 0
  sound    = "magic"

  lifecycle {
    ignore_changes = all
  }
}

# --- Pattern 2: Version-gated re-send via idempotency_key ---
# Content may change freely in HCL; only rotating app_version re-sends.
resource "pushover_message" "release" {
  user_key        = var.pushover_user_key
  message         = "Release ${var.app_version} is live."
  title           = "Release ${var.app_version}"
  priority        = 1
  idempotency_key = var.app_version

  lifecycle {
    ignore_changes = [
      message,
      title,
      priority,
      sound,
      device,
      url,
      url_title,
      html,
      monospace,
      ttl,
      timestamp,
      retry,
      expire,
      callback,
      api_token,
      user_key,
    ]
  }
}

# --- Pattern 3: Explicit trigger resource ---
# Re-send only when deployment_id changes, independent of message text edits.
resource "terraform_data" "notify_trigger" {
  input = var.deployment_id
}

resource "pushover_message" "deploy" {
  user_key = var.pushover_user_key
  message  = "Deployment ${var.deployment_id} finished."
  title    = "Deployment Complete"
  priority = 1

  lifecycle {
    replace_triggered_by = [terraform_data.notify_trigger]
    ignore_changes       = all
  }
}

output "bootstrap_request_id" {
  description = "Request ID from the one-shot bootstrap notification."
  value       = pushover_message.bootstrap.request_id
}

output "release_idempotency_key" {
  description = "Idempotency key currently recorded for the release notification."
  value       = pushover_message.release.idempotency_key
}
