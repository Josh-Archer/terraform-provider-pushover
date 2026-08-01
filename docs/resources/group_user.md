---
page_title: "pushover_group_user Resource - pushover"
subcategory: ""
description: |-
  Adds a user to a Pushover delivery group and manages their membership settings.
---

# pushover_group_user (Resource)

Adds a Pushover user to a delivery group. The group must already exist (create it in the [Pushover dashboard](https://pushover.net) or via your Pushover subscription API). This resource manages a single group–user relationship.

Changing `group_key`, `user_key`, or `device` forces a new resource. Changing `memo` or `disabled` is updated in-place.

## Drift and refresh behavior

On every plan/refresh, Terraform re-reads the group's membership list from the Pushover API and reconciles state:

| External change | Refresh result | Next apply |
| --- | --- | --- |
| User removed from the group outside Terraform | Resource is **removed from state** (destroyed in Terraform terms) | Membership is **recreated** if still in configuration |
| Group deleted / no longer readable as a group | Resource is **removed from state** | Membership is **recreated** (requires the group to exist again) |
| `disabled` toggled outside Terraform | State `disabled` is updated to the remote value | Plan shows a change back to the configured value (if different) |
| `memo` changed outside Terraform | State `memo` is updated to the remote value | Plan shows a change back to the configured value (if different) |

Membership matching is exact on `user_key` and `device`: a device-scoped member is distinct from the same user without a device restriction.

## Example Usage

### Basic membership

```terraform
resource "pushover_group_user" "ops_on_call" {
  group_key = var.pushover_group_key
  user_key  = var.pushover_user_key
  memo      = "Primary on-call engineer"
}
```

### Device-scoped membership

```terraform
resource "pushover_group_user" "mobile_only" {
  group_key = var.pushover_group_key
  user_key  = var.pushover_user_key
  device    = "iphone"
  memo      = "Receives on-call alerts on iPhone only"
}
```

### Managing multiple members

```terraform
variable "on_call_users" {
  type = map(object({
    user_key = string
    memo     = string
  }))
  default = {
    alice = { user_key = "uAliceKey", memo = "Primary"   }
    bob   = { user_key = "uBobKey",   memo = "Secondary" }
  }
}

resource "pushover_group_user" "team" {
  for_each  = var.on_call_users
  group_key = var.pushover_group_key
  user_key  = each.value.user_key
  memo      = each.value.memo
}
```

### Temporarily disabling a member

```terraform
resource "pushover_group_user" "engineer" {
  group_key = var.pushover_group_key
  user_key  = var.pushover_user_key
  disabled  = var.engineer_on_leave   # set to true during leave
}
```

## Schema

### Required

- `group_key` (String) — The Pushover delivery group key. **(Forces replacement)**
- `user_key` (String) — The Pushover user key to add to the group. **(Forces replacement)**

### Optional

- `device` (String) — Restrict notifications to this specific device for the user. **(Forces replacement)**
- `disabled` (Boolean) — Set to `true` to disable notifications without removing the user from the group. Default: `false`.
- `memo` (String) — A note about this group member (≤ 200 characters).

### Read-Only

- `id` (String) — Computed unique identifier: `group_key/user_key` or `group_key/user_key/device` when a device is specified.

## Import

Existing group memberships can be imported using `group_key/user_key` or `group_key/user_key/device`:

```shell
# Membership for all of a user's devices (no device restriction)
terraform import pushover_group_user.ops_on_call gYourGroupKey/uYourUserKey

# Device-scoped membership
terraform import pushover_group_user.mobile_only gYourGroupKey/uYourUserKey/iphone
```

After import, run `terraform plan` (or refresh) so the provider can read the remote `disabled` and `memo` values. If the imported user is not currently a member of the group, refresh removes the resource from state and the next plan will propose creating the membership again.
