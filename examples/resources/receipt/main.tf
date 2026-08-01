# --- Variables ---
variable "pushover_api_token" {
  description = "Pushover application API token."
  type        = string
  sensitive   = true
}

variable "pushover_user_key" {
  description = "Pushover user or group key to send emergency notifications to."
  type        = string
  sensitive   = true
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

# --- Emergency message ---
# Priority 2 requires retry + expire. Pushover returns a receipt token used to
# poll acknowledgement status and to cancel retries when the incident resolves.
resource "pushover_message" "outage" {
  user_key = var.pushover_user_key
  message  = "Production database is DOWN!"
  title    = "CRITICAL OUTAGE"
  priority = 2
  retry    = 60    # Resend every minute until acknowledged or cancelled
  expire   = 3600  # Give up after 1 hour if still open
  callback = "https://ops.example.com/webhook/ack"
}

# --- Receipt lifecycle ---
# Tracks delivery/ack status. Destroying this resource cancels outstanding
# emergency retries (cancel_on_destroy defaults to true).
resource "pushover_receipt" "outage" {
  receipt = pushover_message.outage.receipt
}

output "receipt_id" {
  description = "Emergency receipt token."
  value       = pushover_receipt.outage.receipt
}

output "acknowledged" {
  description = "Whether any recipient has acknowledged the emergency."
  value       = pushover_receipt.outage.acknowledged
}

output "expired" {
  description = "Whether the emergency retry window has expired."
  value       = pushover_receipt.outage.expired
}
