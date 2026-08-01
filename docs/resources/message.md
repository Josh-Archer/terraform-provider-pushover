---
page_title: "pushover_message Resource - pushover"
subcategory: ""
description: |-
  Sends a Pushover push notification. The notification is delivered when this resource is created.
---

# pushover_message (Resource)

Sends a push notification via [Pushover](https://pushover.net) when this resource is created.

Because a sent message cannot be retrieved or deleted via the API, this resource models the *act of sending* rather than a persistent object. **Any change to a configuration attribute forces replacement** and re-sends the message with the new values.

Without lifecycle guardrails, unrelated plan churn (dynamic interpolations, copy edits, provider upgrades that surface attribute diffs, etc.) can accidentally re-send notifications. Prefer the safe patterns below for one-shot and versioned notifies.

## Safe lifecycle patterns

### One-shot notify (`ignore_changes = all`)

Send once on create, then never re-send on subsequent plans—even if attributes in config drift:

```terraform
resource "pushover_message" "bootstrap" {
  user_key = var.pushover_user_key
  message  = "Cluster bootstrap finished."
  title    = "Bootstrap"

  lifecycle {
    ignore_changes = all
  }
}
```

Use this for fire-and-forget alerts that must not fire again when the rest of the stack changes.

### Controlled re-send with `idempotency_key`

Keep content free to edit in HCL while only re-sending when you intentionally rotate a key (for example a release version):

```terraform
resource "pushover_message" "release" {
  user_key        = var.pushover_user_key
  message         = "Release ${var.app_version} is live."
  title           = "Release"
  idempotency_key = var.app_version

  lifecycle {
    ignore_changes = [
      message,
      title,
      priority,
      sound,
      device,
      url,
      url_title,
      html,
      monospace,
      ttl,
      timestamp,
      retry,
      expire,
      callback,
      api_token,
      user_key,
    ]
  }
}
```

`idempotency_key` is stored only in Terraform state and is **not** sent to the Pushover API. Changing it forces replacement; with the `ignore_changes` list above, edits to message body/title/etc. alone produce a no-op plan.

### Explicit re-send with `replace_triggered_by`

Re-send only when another resource or value changes, without tying replacement to every message field:

```terraform
resource "terraform_data" "notify_trigger" {
  input = var.deployment_id
}

resource "pushover_message" "deploy" {
  user_key = var.pushover_user_key
  message  = "Deploy ${var.deployment_id} completed."
  title    = "Deploy"

  lifecycle {
    replace_triggered_by = [terraform_data.notify_trigger]
    ignore_changes       = all
  }
}
```

### Manual re-send

To force a re-send without changing configuration:

```bash
terraform apply -replace=pushover_message.bootstrap
# or (legacy): terraform taint pushover_message.bootstrap && terraform apply
```

See also the [one-shot example module](../../examples/resources/message_oneshot/).

## Example Usage

### Basic notification

```terraform
resource "pushover_message" "hello" {
  user_key = var.pushover_user_key
  message  = "Hello from Terraform!"
}
```

### Rich notification

```terraform
resource "pushover_message" "deploy" {
  user_key  = var.pushover_user_key
  message   = "Deploy of <b>my-app v2.3.0</b> completed in 42 s."
  title     = "✅ Deploy Complete"
  url       = "https://my-app.example.com"
  url_title = "Open Application"
  priority  = 1
  sound     = "magic"
  html      = true
}
```

### Emergency notification

```terraform
resource "pushover_message" "outage" {
  user_key = var.pushover_user_key
  message  = "Production database is unreachable!"
  title    = "🔴 Database Outage"
  priority = 2
  retry    = 60     # Resend every 60 seconds…
  expire   = 3600   # …for up to 1 hour
  callback = "https://ops.example.com/webhook/ack"
}

# Track acknowledgement and cancel retries when this resource is destroyed.
resource "pushover_receipt" "outage" {
  receipt = pushover_message.outage.receipt
}

output "outage_acknowledged" {
  description = "Whether the emergency notification has been acknowledged."
  value       = pushover_receipt.outage.acknowledged
}
```

### Send to a specific device

```terraform
resource "pushover_message" "targeted" {
  user_key = var.pushover_user_key
  message  = "This message goes to your iPhone only."
  device   = "iphone"
}
```

### Message with image attachment

Attachments are uploaded as binary image data via the Pushover API (`multipart/form-data`). The provider accepts either a **local file path** or a **remote `http(s)` URL**. Remote URLs are downloaded by the provider first; Pushover does not fetch URLs itself.

**Limits (enforced by the Pushover API):**

| Constraint | Limit |
| --- | --- |
| Max size | **5,242,880 bytes (5 MiB)** |
| Count | **One** attachment per message |
| Types | Image formats supported by Pushover clients (JPEG, PNG, GIF, WebP, etc.) |

```terraform
# Local file
resource "pushover_message" "with_local_image" {
  user_key        = var.pushover_user_key
  message         = "Build artifact screenshot"
  title           = "CI Result"
  attachment      = "${path.module}/screenshots/result.png"
  attachment_type = "image/png" # optional; inferred from extension when omitted
}

# Remote URL (downloaded by the provider, then uploaded)
resource "pushover_message" "with_remote_image" {
  user_key   = var.pushover_user_key
  message    = "Latest status graph"
  attachment = "https://example.com/status/graph.png"
}
```

## Schema

### Required

- `user_key` (String) — The Pushover user or group key to deliver the message to. **(Forces replacement)**
- `message` (String) — The message body (1–1024 characters). Supports HTML when `html = true`. **(Forces replacement)**

### Optional

- `api_token` (String, Sensitive) — Override the provider-level API token for this message. **(Forces replacement)**
- `attachment` (String) — Local filesystem path or remote `http(s)` URL of an image to attach. Remote URLs are downloaded by the provider and uploaded to Pushover. Max **5,242,880 bytes (5 MiB)**; one attachment per message. **(Forces replacement)**
- `attachment_type` (String) — Optional MIME type for the attachment (e.g. `image/jpeg`). Inferred from the file extension or response `Content-Type` when omitted. Requires `attachment`. **(Forces replacement)**
- `callback` (String) — URL to ping when an emergency (`priority = 2`) message has been acknowledged. **(Forces replacement)**
- `device` (String) — Deliver only to this named device, instead of all of the user's devices. **(Forces replacement)**
- `expire` (Number) — For emergency priority: stop re-sending after this many seconds. Range: 1–10800. **(Forces replacement)**
- `html` (Boolean) — Enable HTML formatting in the message body. **(Forces replacement)**
- `idempotency_key` (String) — Opaque key that forces replacement when changed. Combine with `lifecycle.ignore_changes` on content attributes so only intentional key updates re-send. Not sent to the API. **(Forces replacement)**
- `monospace` (Boolean) — Display the message in a monospace font. **(Forces replacement)**
- `priority` (Number) — Message priority. One of: `-2` (lowest), `-1` (low), `0` (normal, default), `1` (high), `2` (emergency). **(Forces replacement)**
- `retry` (Number) — For emergency priority: resend interval in seconds. Minimum: 30. **(Forces replacement)**
- `sound` (String) — Notification sound key. Use the `pushover_sounds` data source to list valid values. **(Forces replacement)**
- `timestamp` (Number) — Unix timestamp to display instead of the receipt time. **(Forces replacement)**
- `title` (String) — Message title (≤ 250 characters). Defaults to the application name. **(Forces replacement)**
- `ttl` (Number) — Seconds after which Pushover deletes the message from its servers. Minimum: 1. **(Forces replacement)**
- `url` (String) — Supplementary URL (≤ 512 characters). **(Forces replacement)**
- `url_title` (String) — Label for the supplementary URL (≤ 100 characters). **(Forces replacement)**

### Read-Only

- `receipt` (String) — For emergency messages: receipt token. Pass to `pushover_receipt` to track status and cancel on resolve.
- `request_id` (String) — The unique request ID returned by the Pushover API.

## Import

`pushover_message` resources cannot be imported because Pushover does not expose a message-retrieval API.
