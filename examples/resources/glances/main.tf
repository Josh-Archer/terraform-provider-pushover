# --- Variables ---
variable "pushover_api_token" {
  description = "Pushover application API token."
  type        = string
  sensitive   = true
}

variable "pushover_user_key" {
  description = "Pushover user key whose glance widget(s) will be updated."
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

# --- Example 1: Simple text status ---
resource "pushover_glances" "garage" {
  user_key = var.pushover_user_key
  text     = "Garage door open"
}

# --- Example 2: Full sales counter widget ---
resource "pushover_glances" "sales" {
  user_key    = var.pushover_user_key
  title       = "Widgets Sold"
  text        = "42 today"
  subtext     = "Goal: 100"
  badge_count = 42
  percent     = 42
}

# --- Example 3: Device-targeted count complication ---
resource "pushover_glances" "tickets" {
  user_key    = var.pushover_user_key
  device      = "iphone"
  title       = "Open Tickets"
  badge_count = 7
}

output "sales_request_id" {
  description = "Pushover request ID for the sales glance update."
  value       = pushover_glances.sales.request_id
}
