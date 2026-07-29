---
page_title: "pushover_glances Resource - pushover"
subcategory: ""
description: |-
  Updates a Pushover Glances widget (smartwatch complication / lock screen) with short text or numeric data.
---

# pushover_glances (Resource)

Updates a [Pushover Glances](https://pushover.net/api/glances) widget — for example an Apple Watch complication — with short text or numeric data.

Glance updates do **not** produce a push notification alert or sound. They store low-priority state on a constantly-updated screen. The API retains each field until it is overwritten or cleared (send an empty value).

At least one of `title`, `text`, `subtext`, `count`, or `percent` must be set. When an attribute is removed from configuration on update, or when the resource is destroyed, that field is cleared on the widget.

> **Apple Watch note:** Throttle updates (Pushover recommends at least 20 minutes between calls). WatchOS may stop processing updates if you exceed roughly 50 updates per day.

## Example Usage

### Basic text glance

```terraform
resource "pushover_glances" "status" {
  user_key = var.pushover_user_key
  text     = "Garage door open"
}
```

### Full widget update

```terraform
resource "pushover_glances" "sales" {
  user_key = var.pushover_user_key
  device   = "iphone"
  title    = "Widgets Sold"
  text     = "42 today"
  subtext  = "Goal: 100"
  count    = 42
  percent  = 42
}
```

### Count-only complication

```terraform
resource "pushover_glances" "open_tickets" {
  user_key = var.pushover_user_key
  title    = "Open Tickets"
  count    = 7
}
```

## Schema

### Required

- `user_key` (String) — The Pushover user key whose widget(s) should receive the glance data. **(Forces replacement)**

### Optional

- `api_token` (String, Sensitive) — Override the provider-level API token for this resource.
- `count` (Number) — Integer count shown on smaller screens. May be negative.
- `device` (String) — Restrict the update to the widget on this device name. **(Forces replacement)**
- `percent` (Number) — Progress value from 0 through 100 (inclusive).
- `subtext` (String) — Secondary line of data (≤ 100 characters).
- `text` (String) — Main line of data (≤ 100 characters).
- `title` (String) — Description of the data being shown (≤ 100 characters).

### Read-Only

- `id` (String) — Unique identifier (`user_key` or `user_key/device`).
- `request_id` (String) — Request ID returned by the most recent Glances API call.

## Import

`pushover_glances` resources cannot be imported because Pushover does not expose a glance-retrieval API.
