---
page_title: "pushover_group Resource - pushover"
subcategory: ""
description: |-
  Manages the name of an existing Pushover delivery group.
---

# pushover_group (Resource)

Manages the name of an existing Pushover delivery group. Create the group first in the Pushover dashboard or through the Pushover API. This resource renames that group when created and when `name` changes.

Destroying the resource only stops Terraform from managing the group. Pushover does not provide a group deletion endpoint, so the group and its members remain in Pushover.

## Example Usage

```terraform
resource "pushover_group" "operations" {
  group_key = var.pushover_group_key
  name      = "Operations"
}

resource "pushover_group_user" "on_call" {
  group_key = pushover_group.operations.group_key
  user_key  = var.pushover_user_key
}
```

## Schema

### Required

- `group_key` (String) — Key of the existing Pushover delivery group. **(Forces replacement)**
- `name` (String) — Desired group name. Changes rename the group in place.

### Read-Only

- `id` (String) — Same as `group_key`.

## Import

Import an existing group using its group key. Terraform reads the current group name after import.

```shell
terraform import pushover_group.operations gYourGroupKey
```
