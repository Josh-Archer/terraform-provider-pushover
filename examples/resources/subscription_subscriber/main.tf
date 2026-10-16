# --- Variables ---
variable "pushover_api_token" {
  description = "Pushover application API token. The application must own the subscription."
  type        = string
  sensitive   = true
}

variable "pushover_subscription_code" {
  description = "Subscription code from the Pushover dashboard (the code in https://pushover.net/subscribe/CODE)."
  type        = string
}

variable "pushover_user_key" {
  description = "Existing Pushover user key to migrate onto the subscription."
  type        = string
  sensitive   = true
}

variable "on_call_user_key" {
  description = "Another user key to migrate with a device and sound."
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

# --- Example 1: Migrate a user and keep the subscription user key ---
resource "pushover_subscription_subscriber" "alice" {
  subscription = var.pushover_subscription_code
  user_key     = var.pushover_user_key
}

# --- Example 2: Device and sound chosen at migration time ---
resource "pushover_subscription_subscriber" "on_call" {
  subscription = var.pushover_subscription_code
  user_key     = var.on_call_user_key
  device_name  = "iphone"
  sound        = "pushover"
}

# Messages to this subscriber use the key Pushover returned, not the original user key.
resource "pushover_message" "welcome" {
  user_key = pushover_subscription_subscriber.alice.subscribed_user_key
  message  = "You are subscribed to ops alerts."

  lifecycle {
    ignore_changes = all
  }
}

output "alice_subscribed_user_key" {
  description = "Subscription user key to store in place of the original user key."
  value       = pushover_subscription_subscriber.alice.subscribed_user_key
  sensitive   = true
}
