---
page_title: "pushover_license_assignment Resource - pushover"
subcategory: ""
description: |-
  Permanently assigns one pre-paid Pushover license credit to a user account or email address.
---

# pushover_license_assignment (Resource)

Permanently assigns one pre-paid [Pushover license credit](https://pushover.net/api/licensing) to a Pushover user account (`user_key`) or email address (`email`) by calling `POST /1/licenses/assign.json`.

Before using this resource, purchase license credits for your application in the [Pushover dashboard](https://pushover.net/dashboard#apps).

## Cost and Billing Semantics

- **Spends 1 credit on create:** Creating this resource permanently deducts **one pre-paid license credit** from your Pushover application's balance.
- **Non-refundable and irreversible:** Purchased license credits are non-refundable. Once a license is assigned to a Pushover user account or pending email address, it **cannot be revoked or reassigned** to another user.
- **Replacement spends another credit:** Changing `user_key`, `email`, `os`, or `api_token` forces replacement (`RequiresReplace`), which makes a new `POST /1/licenses/assign.json` call and consumes an additional license credit.
- **No network call during plan or refresh:** Assignment only runs in `Create` during `terraform apply`. `Read` makes no network call so `terraform plan` and `terraform refresh` never assign credits or trigger drift replacements.
- **Destroy is a state-only no-op with warning:** Because Pushover has no API to revoke or refund an assigned license, destroying this resource makes no API call, removes the resource from Terraform state only, and emits a warning diagnostic.
- **Never automatically retried:** Per Pushover's Licensing API guidance, failed assignment calls are never automatically retried so transient errors cannot consume multiple credits.

## Example Usage

### Assign a license to an existing Pushover user key

```terraform
data "pushover_licenses" "pool" {}

resource "pushover_license_assignment" "alice" {
  user_key = var.pushover_user_key

  lifecycle {
    precondition {
      condition     = data.pushover_licenses.pool.credits > 0
      error_message = "No Pushover license credits remaining on this application."
    }
  }
}

output "credits_after_alice" {
  value = pushover_license_assignment.alice.credits
}
```

### Assign a platform-specific license by email address

```terraform
resource "pushover_license_assignment" "bob_ios" {
  email = "bob@example.com"
  os    = "iOS"
}
```

## Schema

### Optional

- `user_key` (String) — Pushover user key of the recipient. Exactly one of `user_key` or `email` must be set. **(Forces replacement — consumes another credit)**
- `email` (String) — Email address of the recipient. If the user has not yet registered a Pushover account with this email, the license credit is immediately deducted and held as pending until they sign up. Exactly one of `user_key` or `email` must be set. **(Forces replacement — consumes another credit)**
- `os` (String) — Optional platform restriction: `Android`, `iOS`, or `Desktop`. When omitted, the license applies to the first unlicensed platform on which the user registers a device. **(Forces replacement — consumes another credit)**
- `api_token` (String, Sensitive) — Override the provider-level Pushover application API token. The token must belong to the application that owns the pre-paid license credits. **(Forces replacement — consumes another credit)**

### Read-Only

- `id` (String) — Unique identifier for this license assignment (`user:<user_key>[/<os>]` or `email:<email>[/<os>]`).
- `credits` (Number) — Number of unassigned license credits remaining on the application immediately after this assignment.
- `request_id` (String) — Request ID from the license assignment call.

## Import

`pushover_license_assignment` cannot be imported. The Pushover Licensing API has no endpoint to look up an existing license assignment without assigning a new license credit.
