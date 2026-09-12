---
page_title: "pushover_limits Data Source - pushover"
subcategory: ""
description: |-
  Retrieves monthly message limits, remaining quota, and reset time for a Pushover application.
---

# pushover_limits (Data Source)

Retrieves the monthly message quota, number of remaining messages, and next quota reset time for the configured Pushover application from the Pushover API (`GET /1/apps/limits.json`).

This data source is useful for:
- Checking remaining message allowance before running batch notifications.
- Emitting monitoring alerts or preventing deployments when quota is nearly exhausted.
- Querying quota status across multiple applications using the optional `api_token` override.

## Example Usage

### Read quota for default provider application

```terraform
data "pushover_limits" "current" {}

output "remaining_messages" {
  value = data.pushover_limits.current.remaining
}

output "quota_resets_at" {
  value = data.pushover_limits.current.reset
}
```

### Precondition check before triggering alerts

```terraform
data "pushover_limits" "app" {}

resource "pushover_message" "deploy_notice" {
  user_key = var.pushover_user_key
  message  = "Deployment succeeded."

  lifecycle {
    precondition {
      condition     = data.pushover_limits.app.remaining > 50
      error_message = "Pushover message quota has fewer than 50 messages remaining."
    }
  }
}
```

### Check limits for a specific application token

```terraform
data "pushover_limits" "secondary" {
  api_token = var.secondary_app_token
}
```

## Schema

### Optional

- `api_token` (String, Sensitive) — Override the provider-level API token for querying limits.

### Read-Only

- `id` (String) — Resource identifier.
- `limit` (Number) — Total monthly message quota for this application.
- `remaining` (Number) — Messages remaining in the current monthly period.
- `reset` (Number) — Unix timestamp indicating when the message quota will reset.
