---
page_title: "pushover_subscription_subscriber Resource - pushover"
subcategory: ""
description: |-
  Migrates an existing Pushover user key onto an application subscription and stores the returned subscription user key.
---

# pushover_subscription_subscriber (Resource)

Migrates an existing Pushover user onto an application [subscription](https://pushover.net/api/subscriptions) by calling `POST /1/subscriptions/migrate.json`. The subscription must already exist in the [Pushover dashboard](https://pushover.net). Create the subscription there, then copy its subscription code (the code in `https://pushover.net/subscribe/CODE`).

On success Pushover returns a `subscribed_user_key`. Use that key in place of the original user key when sending messages to this subscriber. You can discard the original user key for this subscription. Group-backed subscriptions can also be addressed with the delivery group key.

Pushover does not offer an API to list subscribers or to unsubscribe someone. **Destroying this resource removes it from Terraform state only.** The person stays subscribed until they unsubscribe themselves.

Changing `subscription`, `user_key`, `device_name`, `sound`, or `api_token` forces a new migration. Replacement does not remove the previous subscription.

## Drift and refresh behavior

On refresh, the provider validates `subscribed_user_key` with the users API. It does not read `device_name` or `sound` back, because the migration API does not return them.

| External change | Refresh result | Next apply |
| --- | --- | --- |
| Subscribed user key deleted (user unsubscribed) | Resource is **removed from state** | Migration is **run again** if still in configuration |
| Other API errors (bad token, HTTP 5xx, generic not-found) | Refresh **fails** and state is kept | No new migration until refresh succeeds |
| Device or sound changed in the Pushover dashboard | Not detectable | No plan diff |

## Example Usage

### Migrate one user

```terraform
resource "pushover_subscription_subscriber" "alice" {
  subscription = var.pushover_subscription_code
  user_key     = var.pushover_user_key
}

resource "pushover_message" "welcome" {
  user_key = pushover_subscription_subscriber.alice.subscribed_user_key
  message  = "You are subscribed to ops alerts."
  lifecycle {
    ignore_changes = all
  }
}
```

### Limit delivery to one device and sound

```terraform
resource "pushover_subscription_subscriber" "on_call" {
  subscription = var.pushover_subscription_code
  user_key     = var.pushover_user_key
  device_name  = "iphone"
  sound        = "pushover"
}
```

## Schema

### Required

- `subscription` (String) — Subscription code from the Pushover dashboard. **(Forces replacement)**
- `user_key` (String) — Existing Pushover user key to migrate. **(Forces replacement)**

### Optional

- `device_name` (String) — Limit the subscription to this device (`[A-Za-z0-9_-]`, 1–25 characters). Maps to the API parameter `device_name`. **(Forces replacement)**
- `sound` (String) — Preferred default sound for this subscriber. Omit to keep the user's default. **(Forces replacement)**
- `api_token` (String, Sensitive) — Override the provider-level application API token. The token must belong to the application that owns the subscription. **(Forces replacement)**

### Read-Only

- `id` (String) — `subscription/user_key` or `subscription/user_key/device_name`.
- `subscribed_user_key` (String) — Subscription user key returned by Pushover. Use this when sending messages to the subscriber.
- `request_id` (String) — Request ID from the migration call.

## Import

`pushover_subscription_subscriber` cannot be imported. The Subscription API can create a migration but cannot look up an existing subscriber without calling that migration again.
