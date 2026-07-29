---
page_title: "pushover_message Resource - pushover"
subcategory: ""
description: |-
  Sends a Pushover push notification. The notification is delivered when this resource is created.
---

# pushover_message (Resource)

Sends a push notification via [Pushover](https://pushover.net) when this resource is created.

Because a sent message cannot be retrieved or deleted via the API, this resource models the *act of sending* rather than a persistent object. Any change to an attribute that is marked `(Forces replacement)` will destroy the existing resource and re-send the message with the new values.

To re-send a message without changing its content, use `terraform taint pushover_message.<name>` or add a `lifecycle { replace_triggered_by = [...] }` block referencing another resource.

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

output "outage_receipt" {
  description = "Use this receipt to poll or cancel the emergency notification."
  value       = pushover_message.outage.receipt
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
- `html` (Boolean) — Enable HTML formatting in the message body.
- `monospace` (Boolean) — Display the message in a monospace font.
- `priority` (Number) — Message priority. One of: `-2` (lowest), `-1` (low), `0` (normal, default), `1` (high), `2` (emergency).
- `retry` (Number) — For emergency priority: resend interval in seconds. Minimum: 30. **(Forces replacement)**
- `sound` (String) — Notification sound key. Use the `pushover_sounds` data source to list valid values. **(Forces replacement)**
- `timestamp` (Number) — Unix timestamp to display instead of the receipt time.
- `title` (String) — Message title (≤ 250 characters). Defaults to the application name. **(Forces replacement)**
- `ttl` (Number) — Seconds after which Pushover deletes the message from its servers. Minimum: 1.
- `url` (String) — Supplementary URL (≤ 512 characters). **(Forces replacement)**
- `url_title` (String) — Label for the supplementary URL (≤ 100 characters). **(Forces replacement)**

### Read-Only

- `receipt` (String) — For emergency messages: receipt token for polling acknowledgement status.
- `request_id` (String) — The unique request ID returned by the Pushover API.

## Import

`pushover_message` resources cannot be imported because Pushover does not expose a message-retrieval API.
