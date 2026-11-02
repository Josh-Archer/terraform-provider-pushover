# --- Variables ---
variable "pushover_api_token" {
  description = "Pushover application API token. Required to configure the provider. Team calls do not send this value."
  type        = string
  sensitive   = true
}

variable "pushover_team_token" {
  description = "Team API token from the Pushover team settings page."
  type        = string
  sensitive   = true
}

variable "bob_initial_password" {
  description = "Initial password for Bob. Stored in Terraform state. Leave null to let Pushover generate a password and email it."
  type        = string
  sensitive   = true
  default     = null
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

# --- Example 1: Add a team member ---
resource "pushover_team_user" "ada" {
  token = var.pushover_team_token
  email = "ada@example.com"
  name  = "Ada Lovelace"
}

# --- Example 2: Administrator placed in a named delivery group ---
# group is the delivery group name. Pushover creates the group when it is missing.
resource "pushover_team_user" "ops_admin" {
  token   = var.pushover_team_token
  email   = "ops@example.com"
  name    = "Ops Admin"
  admin   = true
  instant = true
  group   = "Ops"
}

# --- Example 3: Set an initial password instead of emailing a generated one ---
resource "pushover_team_user" "bob" {
  token    = var.pushover_team_token
  email    = "bob@example.com"
  name     = "Bob Singh"
  password = var.bob_initial_password
}

output "ada_user_id" {
  description = "Pushover user id from the team listing."
  value       = pushover_team_user.ada.user_id
}
