---
page_title: "pushover_receipt Resource - pushover"
subcategory: ""
description: |-
  Tracks an emergency (priority 2) Pushover message receipt and cancels outstanding retries when destroyed.
---

# pushover_receipt (Resource)

Tracks an emergency (`priority = 2`) [Pushover](https://pushover.net) message receipt and manages cancel-on-resolve lifecycle.

When you send an emergency message with `pushover_message`, Pushover returns a `receipt` token and keeps re-notifying recipients until someone acknowledges, the `expire` window ends, or the receipt is cancelled. This resource:

1. **Tracks** acknowledgement, delivery, expiry, and callback status on each Terraform refresh/read.
2. **Cancels** outstanding retries when the resource is destroyed (default), so resolving an incident in Terraform stops the alert noise.

## Example Usage

### Emergency message with cancel-on-destroy

```terraform
resource "pushover_message" "outage" {
  user_key = var.pushover_user_key
  message  = "Production database is unreachable!"
  title    = "Database Outage"
  priority = 2
  retry    = 60
  expire   = 3600
  callback = "https://ops.example.com/webhook/ack"
}

resource "pushover_receipt" "outage" {
  receipt = pushover_message.outage.receipt
  # cancel_on_destroy defaults to true: terraform destroy (or replace) cancels retries
}

output "acknowledged" {
  value = pushover_receipt.outage.acknowledged
}
```

### Track without cancelling on destroy

```terraform
resource "pushover_receipt" "observe_only" {
  receipt           = var.emergency_receipt
  cancel_on_destroy = false
}
```

### Incident-style lifecycle

Keep the receipt resource as long as the incident is open. When the incident is closed, remove the resource (or destroy the stack) so Terraform cancels the emergency notification:

```terraform
resource "pushover_message" "incident" {
  count = var.incident_open ? 1 : 0

  user_key = var.pushover_user_key
  message  = var.incident_summary
  title    = "Open incident"
  priority = 2
  retry    = 60
  expire   = 10800
}

resource "pushover_receipt" "incident" {
  count = var.incident_open ? 1 : 0

  receipt = pushover_message.incident[0].receipt
}
```

## Schema

### Required

- `receipt` (String) — Emergency receipt token from `pushover_message.receipt` (`priority = 2`). **(Forces replacement)**

### Optional

- `cancel_on_destroy` (Boolean) — When `true` (default), destroying this resource cancels outstanding emergency retries via the Pushover cancel API. Set to `false` to drop tracking without cancelling.

### Read-Only

- `id` (String) — Same as `receipt`.
- `acknowledged` (Boolean) — `true` if any recipient acknowledged the emergency message.
- `acknowledged_at` (Number) — Unix timestamp of first acknowledgement, or `0`.
- `acknowledged_by` (String) — User key of the acknowledging recipient, if any.
- `acknowledged_by_device` (String) — Device that acknowledged the message, if any.
- `last_delivered_at` (Number) — Unix timestamp of the most recent delivery attempt.
- `expired` (Boolean) — `true` if the retry window has ended.
- `expires_at` (Number) — Unix timestamp when the emergency expires (or expired).
- `called_back` (Boolean) — `true` if the optional callback URL was invoked.
- `called_back_at` (Number) — Unix timestamp of the callback, or `0`.
- `request_id` (String) — Request ID from the most recent receipts API call.

## Import

Import is not supported. Create the resource with the receipt token from an emergency `pushover_message`.
