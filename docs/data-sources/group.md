---
page_title: "pushover_group Data Source - pushover"
subcategory: ""
description: |-
  Retrieves details about a Pushover delivery group, including its name and member list.
---

# pushover_group (Data Source)

Retrieves details about an existing Pushover delivery group from the Pushover API, including its display name and current list of members.

This data source is useful for:
- Querying group membership to dynamically route notifications.
- Filtering active vs. disabled members in a delivery group.
- Reading the human-readable name of a group given its group key.

## Example Usage

### Query group details and members

```terraform
data "pushover_group" "devops" {
  group_key = var.pushover_group_key
}

output "group_name" {
  value = data.pushover_group.devops.name
}

output "active_members" {
  value = [for u in data.pushover_group.devops.users : u.user_key if !u.disabled]
}
```

### Query group with an application token override

```terraform
data "pushover_group" "team" {
  group_key = var.pushover_group_key
  api_token = var.app_token
}
```

## Schema

### Required

- `group_key` (String) — The Pushover delivery group key to look up.

### Optional

- `api_token` (String, Sensitive) — Override the provider-level API token for this query.

### Read-Only

- `id` (String) — The delivery group key (same as `group_key`).
- `name` (String) — The name of the delivery group.
- `users` (Attributes List) — The list of members in this delivery group. (see [below for nested schema](#nestedatt--users))

<a id="nestedatt--users"></a>
### Nested Schema for `users`

Read-Only:

- `user_key` (String) — The Pushover user key of the member.
- `device` (String) — The device name restricting delivery to this user, if configured.
- `memo` (String) — A memo or description for this group member.
- `disabled` (Boolean) — Whether delivery to this member is currently disabled.
