# --- Variables ---
variable "pushover_api_token" {
  description = "Pushover application API token."
  type        = string
  sensitive   = true
}

variable "pushover_group_key" {
  description = "Pushover delivery group key."
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

# The group must already exist in Pushover. This resource manages its name.
resource "pushover_group" "managed" {
  group_key = var.pushover_group_key
  name      = "Operations"
}

# --- Example 1: Add a single user to a group ---
resource "pushover_group_user" "alice" {
  group_key = pushover_group.managed.group_key
  user_key  = "uAliceUserKey"
  memo      = "Primary on-call"
}

# --- Example 2: Add a user with a specific device ---
resource "pushover_group_user" "bob_mobile" {
  group_key = pushover_group.managed.group_key
  user_key  = "uBobUserKey"
  device    = "iphone"
  memo      = "Receives alerts on iPhone only"
}

# --- Example 3: Manage multiple users from a variable ---
variable "on_call_roster" {
  description = "Map of user identifier to user key and memo."
  type = map(object({
    user_key = string
    memo     = string
  }))
  default = {
    alice = { user_key = "uAliceKey", memo = "Primary"   }
    bob   = { user_key = "uBobKey",   memo = "Secondary" }
    carol = { user_key = "uCarolKey", memo = "Backup"    }
  }
}

resource "pushover_group_user" "roster" {
  for_each  = var.on_call_roster
  group_key = pushover_group.managed.group_key
  user_key  = each.value.user_key
  memo      = each.value.memo
}

# --- Example 4: Temporarily disable a member ---
variable "engineer_on_leave" {
  description = "Set to true when the engineer is on leave."
  type        = bool
  default     = false
}

resource "pushover_group_user" "engineer" {
  group_key = pushover_group.managed.group_key
  user_key  = "uEngineerKey"
  memo      = "On-call engineer"
  disabled  = var.engineer_on_leave
}
