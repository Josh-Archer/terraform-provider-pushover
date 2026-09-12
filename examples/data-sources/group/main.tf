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

variable "pushover_group_key" {
  type      = string
  sensitive = true
}

# Fetch details for an existing Pushover delivery group.
data "pushover_group" "team" {
  group_key = var.pushover_group_key
}

output "group_name" {
  description = "The name of the Pushover group."
  value       = data.pushover_group.team.name
}

output "active_members" {
  description = "User keys of active (non-disabled) members."
  value       = [for u in data.pushover_group.team.users : u.user_key if !u.disabled]
}
