# Terraform / OpenTofu Provider for Pushover

[![Tests](https://github.com/Josh-Archer/terraform-provider-pushover/actions/workflows/test.yml/badge.svg)](https://github.com/Josh-Archer/terraform-provider-pushover/actions/workflows/test.yml)

The Pushover provider lets you send push notifications and manage delivery groups through [Pushover](https://pushover.net) directly from Terraform or OpenTofu.

## Features

- **Send notifications** (`pushover_message`) – Full Pushover message API including priority levels, sounds, HTML formatting, URL attachments, per-device targeting, TTL, and emergency messages with retry/expire/callback.
- **Manage group membership** (`pushover_group_user`) – Add, remove, enable, or disable users in Pushover delivery groups.
- **Update glance widgets** (`pushover_glances`) – Push short text or numeric data to smartwatch/lock-screen widgets via the Glances API.
- **List available sounds** (`pushover_sounds`) – Query all notification sounds available to your application.
- **Validate recipients** (`pushover_validate_user`) – Verify a user or group key and enumerate its registered devices.
- **Resilient HTTP client** – Bounded retries with exponential backoff on transient HTTP 5xx/429 responses; honors `Retry-After` when present.

## Requirements

| Tool       | Version  |
|------------|----------|
| Go         | ≥ 1.22   |
| Terraform  | ≥ 1.5    |
| OpenTofu   | ≥ 1.6    |

## Quick Start

```hcl
terraform {
  required_providers {
    pushover = {
      source  = "Josh-Archer/pushover"
      version = "~> 1.0"
    }
  }
}

provider "pushover" {
  # Set via PUSHOVER_API_TOKEN env var or inline:
  api_token = var.pushover_api_token
}

resource "pushover_message" "deploy_notification" {
  user_key = var.pushover_user_key
  message  = "Deploy of ${var.app_name} completed successfully."
  title    = "Deployment Complete"
  priority = 1
  sound    = "magic"
  url      = "https://my-app.example.com"
  url_title = "Open Application"
}
```

## Provider Configuration

| Attribute   | Type   | Required | Description |
|-------------|--------|----------|-------------|
| `api_token` | string | Yes*     | Pushover application API token. Can also be set via `PUSHOVER_API_TOKEN`. |

## Resources

### `pushover_message`

Sends a Pushover notification when created. All attributes trigger replacement when changed (the message is re-sent). Use `lifecycle.replace_triggered_by` or `terraform taint` to resend without changing attributes.

```hcl
resource "pushover_message" "alert" {
  user_key  = "uYourUserOrGroupKey"
  message   = "Server CPU is at 95%"
  title     = "⚠️ High CPU Alert"
  priority  = 1
  sound     = "siren"
  url       = "https://grafana.example.com/dashboards"
  url_title = "Open Grafana"
  html      = true
}
```

**Emergency messages** (priority `2`) require `retry` and `expire`:

```hcl
resource "pushover_message" "outage" {
  user_key = "uYourUserOrGroupKey"
  message  = "Production database is DOWN"
  title    = "🔴 OUTAGE"
  priority = 2
  retry    = 60     # Re-send every 60 s until acknowledged
  expire   = 3600   # Stop re-sending after 1 h
  callback = "https://ops.example.com/ack"
}

output "outage_receipt" {
  value = pushover_message.outage.receipt
}
```

#### Attributes

| Attribute    | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `user_key`   | string | ✅        | Pushover user or group key |
| `message`    | string | ✅        | Message body (1–1024 chars; HTML supported) |
| `api_token`  | string | –        | Per-message API token override |
| `title`      | string | –        | Message title (≤ 250 chars) |
| `url`        | string | –        | Supplementary URL (≤ 512 chars) |
| `url_title`  | string | –        | URL label (≤ 100 chars) |
| `priority`   | int    | –        | `-2` lowest · `-1` low · `0` normal · `1` high · `2` emergency |
| `sound`      | string | –        | Notification sound key |
| `device`     | string | –        | Deliver only to this device |
| `timestamp`  | int    | –        | Override message timestamp (Unix) |
| `html`       | bool   | –        | Enable HTML in message body |
| `monospace`  | bool   | –        | Display in monospace font |
| `ttl`        | int    | –        | Seconds before Pushover deletes the message (≥ 1) |
| `retry`      | int    | ✅ if priority=2 | Re-send interval in seconds (≥ 30) |
| `expire`     | int    | ✅ if priority=2 | Stop re-sending after this many seconds (1–10800) |
| `callback`   | string | –        | URL to ping when emergency message is acknowledged |
| `receipt`    | string | computed | Emergency receipt token |
| `request_id` | string | computed | Pushover API request ID |

---

### `pushover_group_user`

Adds a user to an existing Pushover delivery group. The group key must already exist in Pushover (create it in the [Pushover dashboard](https://pushover.net)).

```hcl
resource "pushover_group_user" "ops_team" {
  group_key = "gYourGroupKey"
  user_key  = "uYourUserKey"
  memo      = "On-call engineer"
}
```

#### Attributes

| Attribute   | Type   | Required | Description |
|-------------|--------|----------|-------------|
| `group_key` | string | ✅        | Pushover delivery group key |
| `user_key`  | string | ✅        | Pushover user key |
| `device`    | string | –        | Restrict to a specific device |
| `memo`      | string | –        | Note about this member |
| `disabled`  | bool   | –        | Disable notifications without removing (default: `false`) |
| `id`        | string | computed | `group_key/user_key[/device]` |

---

### `pushover_glances`

Updates a [Pushover Glances](https://pushover.net/api/glances) widget (e.g. Apple Watch complication) with short text or numeric data. This does not send a push notification.

```hcl
resource "pushover_glances" "sales" {
  user_key = "uYourUserKey"
  title    = "Widgets Sold"
  text     = "42 today"
  count    = 42
  percent  = 42
}
```

#### Attributes

| Attribute    | Type   | Required | Description |
|--------------|--------|----------|-------------|
| `user_key`   | string | ✅        | Pushover user key |
| `title`      | string | –*       | Description of the data (≤ 100 chars) |
| `text`       | string | –*       | Main line of data (≤ 100 chars) |
| `subtext`    | string | –*       | Secondary line (≤ 100 chars) |
| `count`      | int    | –*       | Integer count (may be negative) |
| `percent`    | int    | –*       | Progress 0–100 |
| `device`     | string | –        | Restrict to widget on this device |
| `api_token`  | string | –        | Per-resource API token override |
| `id`         | string | computed | `user_key` or `user_key/device` |
| `request_id` | string | computed | Latest Glances API request ID |

\* At least one of `title`, `text`, `subtext`, `count`, or `percent` is required.

---

## Data Sources

### `pushover_sounds`

Returns all notification sounds available to your application.

```hcl
data "pushover_sounds" "available" {}

output "sound_keys" {
  value = data.pushover_sounds.available.keys
}
```

| Attribute | Type        | Description |
|-----------|-------------|-------------|
| `sounds`  | map(string) | Sound key → human-readable name |
| `keys`    | list(string)| List of sound keys (for use in `pushover_message.sound`) |

---

### `pushover_validate_user`

Validates a user or group key and returns registered devices and licenses.

```hcl
data "pushover_validate_user" "recipient" {
  user_key = var.pushover_user_key
}

output "recipient_devices" {
  value = data.pushover_validate_user.recipient.devices
}
```

| Attribute  | Type         | Description |
|------------|--------------|-------------|
| `user_key` | string       | Key to validate |
| `device`   | string       | Optional: filter to specific device |
| `api_token`| string       | Optional: per-request token override |
| `is_group` | bool         | `true` if key belongs to a group |
| `devices`  | list(string) | Registered device names |
| `licenses` | list(string) | Active license types |

---

## Environment Variables

| Variable              | Description |
|-----------------------|-------------|
| `PUSHOVER_API_TOKEN`  | Pushover application API token |
| `PUSHOVER_USER_KEY`   | Used by acceptance tests |

## Development

```bash
# Build
go build ./...

# Run unit tests (no API key required)
go test ./...

# Run acceptance tests (requires real credentials)
PUSHOVER_API_TOKEN=your_token PUSHOVER_USER_KEY=your_key go test ./... -run Acc

# Build release binaries
goreleaser build --snapshot --clean
```

## Publishing

Releases are published automatically by the `release.yml` GitHub Actions workflow when a tag matching `v*` is pushed. You can also run the workflow manually (`workflow_dispatch`) by providing a specific tag via `release_tag` (for example `v0.0.1`).

The release assets are generated in Terraform/OpenTofu registry-compatible format (including `terraform-registry-manifest.json`, checksums, and detached checksum signature).

Required GitHub Actions secrets:

- `GPG_PRIVATE_KEY`: ASCII-armored private key used to sign checksum files.
- `PASSPHRASE`: Passphrase for `GPG_PRIVATE_KEY` (`GPG_PASSPHRASE` is also supported for backward compatibility).

See [`.github/workflows/release.yml`](.github/workflows/release.yml) for the full workflow.
