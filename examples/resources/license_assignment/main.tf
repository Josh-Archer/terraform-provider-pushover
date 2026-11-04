# --- Variables ---
variable "pushover_api_token" {
  description = "Pushover application API token that owns pre-paid license credits."
  type        = string
  sensitive   = true
}

variable "pushover_user_key" {
  description = "Pushover user key of the recipient."
  type        = string
  sensitive   = true
}

variable "employee_email" {
  description = "Email address of a recipient to license on iOS."
  type        = string
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

# --- Check available credits (read-only) ---
data "pushover_licenses" "pool" {}

# --- Example 1: Assign a license by user key (any OS) ---
# WARNING: Creating this resource permanently consumes 1 non-refundable license credit.
# Destroying this resource removes Terraform state only and does not refund the credit.
resource "pushover_license_assignment" "alice" {
  user_key = var.pushover_user_key

  lifecycle {
    precondition {
      condition     = data.pushover_licenses.pool.credits > 0
      error_message = "No Pushover license credits remaining on this application."
    }
  }
}

# --- Example 2: Assign an iOS-specific license by email address ---
resource "pushover_license_assignment" "bob_ios" {
  email = var.employee_email
  os    = "iOS"
}

output "remaining_credits_after_alice" {
  description = "Remaining license credits returned by the assignment call."
  value       = pushover_license_assignment.alice.credits
}
