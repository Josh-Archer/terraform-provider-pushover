---
page_title: "pushover_team_user Resource - pushover"
subcategory: ""
description: |-
  Adds a user to a Pushover team and removes them when the resource is destroyed.
---

# pushover_team_user (Resource)

Adds a user to a [Pushover team](https://pushover.net/api/teams). The team must already exist. Create a team API token on the team settings page and pass it as `token`. That value is the team token, and it is sensitive. The provider `api_token` is still the application token used to configure the provider; team calls do not send it.

Changing any argument forces a new membership. The Teams API has no update method, so Terraform removes the user and adds them again. Removing a user also removes them from the team's delivery groups. At least one administrator must remain on the team.

## Drift and refresh behavior

On every plan/refresh, Terraform re-reads the team listing (`GET /teams.json`) and reconciles state:

| External change | Refresh result | Next apply |
| --- | --- | --- |
| Invitation is still pending (`user_id` is null and user is not in `GET /teams.json` yet) | Resource is **kept in state** as pending (`id = team_token/email`, `user_id = null`) | **No change** (plan settles until the user accepts the invitation) |
| Invited user accepts the invitation and appears in `GET /teams.json` | State adopts `user_id` and updates `id` to `team_token/user_id` | **No change** unless `admin` or `name` differs from configuration |
| Confirmed member (`user_id` already set) removed from the team outside Terraform | Resource is **removed from state** | Membership is **recreated** if it is still in configuration |
| Stored user id is gone, even if another person now uses the same email | Resource is **removed from state** | Membership is **recreated** for the configured email |
| `admin` changed outside Terraform | State `admin` is updated from the listing's `administrator` flag | Plan **replaces** the membership when that differs from configuration |
| `name` changed outside Terraform, and `name` is set in configuration | State `name` is updated | Plan **replaces** the membership when that differs from configuration |
| Team token rejected, or any other listing error | Refresh **fails** and state is kept | No change until the listing succeeds |

`password`, `instant`, and `group` are sent only when the user is added. The listing does not return them, so refresh preserves the prior state values.

If add succeeds but the new user is not in the listing yet (for example, while an email invitation is pending), the resource stays in state with `id = team_token/email` and `user_id = null`. Refresh treats a pending invitation as present so plans remain stable instead of re-inviting on every apply. Once the user joins the team and appears in `GET /teams.json`, refresh records `user_id` and updates `id` to `team_token/user_id`. After `user_id` is set, if the user is later absent from the team listing, refresh removes the resource from state so the next apply recreates the membership. Note that the Pushover Teams API does not expose a pending-invitations listing endpoint, so a pending invitation revoked outside Terraform before the user ever joins cannot be detected by `GET /teams.json`.

## Example Usage

### Basic membership

```terraform
resource "pushover_team_user" "ada" {
  token = var.pushover_team_token
  email = "ada@example.com"
  name  = "Ada Lovelace"
}
```

### Administrator in a delivery group

`group` is the delivery group **name**. Pushover creates that group when it does not already exist. Omit `group` to use the team's default delivery group.

```terraform
resource "pushover_team_user" "ops_admin" {
  token   = var.pushover_team_token
  email   = "ops@example.com"
  name    = "Ops Admin"
  admin   = true
  instant = true
  group   = "Ops"
}
```

### Initial password

`password` is marked `Sensitive` and is sent only on create (`POST /1/teams/add_user.json`); the Pushover Teams API never returns passwords. Because `terraform-plugin-framework` v1.13 (Terraform `< 1.11`) does not support write-only attributes (`WriteOnly`), when `password` is set in configuration Terraform retains the configured value (marked sensitive) in state so subsequent plans settle without replacement diffs, while imported resources leave `password` as `null`. Omit `password` (or use `instant = true`) to let Pushover generate and email a password without storing an initial password in Terraform state.

```terraform
resource "pushover_team_user" "bob" {
  token    = var.pushover_team_token
  email    = "bob@example.com"
  name     = "Bob Singh"
  password = var.bob_initial_password
}
```

## Schema

### Required

- `email` (String) — Email address of the team member. Pushover uses this address to add and remove the user. **(Forces replacement)**
- `token` (String, Sensitive) — Team API token from the team settings page. Pushover does not accept an application API token for team calls. **(Forces replacement)**

### Optional

- `admin` (Boolean) — When true, the user is a team administrator. Refresh copies the `administrator` flag from the team listing. A change replaces the membership. At least one administrator must remain on the team. Defaults to `false`. **(Forces replacement)**
- `group` (String) — Delivery group name to place the user in. Pushover creates the group if it does not exist. Omit to use the team's default delivery group. This is the group name, not a group key. **(Forces replacement)**
- `instant` (Boolean) — When true, the welcome email includes an Instant Login link (valid for 7 days). A change replaces the membership. Defaults to `false`. **(Forces replacement)**
- `name` (String) — Full name stored for this team member. When set, refresh copies a remote rename into state. **(Forces replacement)**
- `password` (String, Sensitive) — Initial password for the new user. Sent only on create; the Pushover Teams API never returns passwords. Because `terraform-plugin-framework` v1.13 (Terraform `< 1.11`) does not support write-only attributes, Terraform retains the configured value (marked sensitive) in state to keep plans stable, while imported resources leave `password` `null`. When omitted, Pushover generates a password and emails it without storing a password in state. **(Forces replacement)**

### Read-Only

- `id` (String, Sensitive) — `team_token/user_id` once the listing returns a user id, or `team_token/email` while an invitation is pending. Sensitive because it contains the team token.
- `user_id` (String) — Pushover user id from the team listing. `null` while an invitation is pending and populated once the user joins the team. Import uses this id.

## Import

Import an existing member with the team token and the user id from the team listing (`users[].id`):

```shell
terraform import pushover_team_user.ada <team_token>/<user_id>
```

`team_token/email` is also accepted when the listing id is not known yet. The import argument contains the team token, so it can remain in shell history. After import, refresh fills `email`, `admin`, and `user_id` from the team listing. `name` is filled only when `name` is already set in state. Set `admin = true` in configuration when the imported user is an administrator. The default is `false`, and a difference replaces the membership.
