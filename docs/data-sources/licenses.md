---
page_title: "pushover_licenses Data Source - pushover"
subcategory: ""
description: |-
  Retrieves the number of remaining pre-paid license credits for a Pushover application.
---

# pushover_licenses (Data Source)

Retrieves the number of unassigned pre-paid [Pushover license credits](https://pushover.net/api/licensing) remaining on the configured Pushover application (`GET /1/licenses.json`).

This data source is read-only and never assigns or consumes license credits. Use it to:
- Check available license credits before assigning licenses with [`pushover_license_assignment`](../resources/license_assignment.md).
- Guard `pushover_license_assignment` resources with a `lifecycle.precondition` so plans fail early if credits are exhausted.
- Monitor remaining license credit balance across multiple applications using the optional `api_token` override.

## Example Usage

### Read remaining license credits for the default provider application

```terraform
data "pushover_licenses" "current" {}

output "remaining_license_credits" {
  value = data.pushover_licenses.current.credits
}
```

### Guard a license assignment with a precondition

```terraform
data "pushover_licenses" "app" {}

resource "pushover_license_assignment" "employee" {
  user_key = var.pushover_user_key
  os       = "iOS"

  lifecycle {
    precondition {
      condition     = data.pushover_licenses.app.credits > 0
      error_message = "Pushover application has no remaining license credits."
    }
  }
}
```

### Check license credits for a specific application token

```terraform
data "pushover_licenses" "secondary" {
  api_token = var.secondary_app_token
}
```

## Schema

### Optional

- `api_token` (String, Sensitive) — Override the provider-level application API token for querying remaining license credits.

### Read-Only

- `id` (String) — Static data source identifier (`licenses`).
- `credits` (Number) — Number of unassigned pre-paid license credits remaining on the application.
- `request_id` (String) — Unique request ID returned by the Pushover Licensing API.
