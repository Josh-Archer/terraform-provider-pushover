# Lifecycle contract coverage

Inventory of create, read, import, drift, and destroy for every Pushover resource. The suite is `internal/provider/lifecycle_contract_test.go` (`TestLifecycleContract` and `TestLifecycleContractSensitiveSchema`).

Each case drives the Terraform CLI through `resource.UnitTest` against an `httptest` double. `PUSHOVER_API_BASE_URL` points the provider at that server. The suite does not call Pushover, does not set `TF_ACC`, and does not need credentials. `TF_ACC_TERRAFORM_PATH` selects the engine. An absolute path to `terraform` or `tofu` both run the same steps. OpenTofu 1.11 rejects the legacy `-` provider namespace on `registry.terraform.io`, so when the binary name starts with `tofu` the suite sets `TF_ACC_PROVIDER_HOST=registry.opentofu.org` unless that variable is already set.

`TestLifecycleContract` fails when a registered resource has no case, or when a case names a resource that is not registered. `TestLifecycleContractSensitiveSchema` fails when a resource gains or drops a sensitive attribute without an update to the golden list. Older unit tests stay in place.

Live acceptance tests remain gated by `TF_ACC` and `PUSHOVER_API_TOKEN`. Tests that assign license credits also require `PUSHOVER_ALLOW_PAID_TESTS=1`. Those tests are not part of this suite.

## How to run

```bash
# Included in the unit suite. Uses Terraform on PATH.
go test ./... -count=1 -timeout 300s

# One engine. The path must be absolute.
TF_ACC_TERRAFORM_PATH="$(command -v terraform)" \
  go test ./internal/provider -count=1 -timeout 300s -run '^TestLifecycleContract'

TF_ACC_TERRAFORM_PATH="$(command -v tofu)" \
  go test ./internal/provider -count=1 -timeout 300s -run '^TestLifecycleContract'
```

`make test-lifecycle`, `make test-lifecycle-terraform`, and `make test-lifecycle-tofu` run the same package filter.

## Phases

| Phase | What the suite asserts |
|---|---|
| Create | Apply stores the attributes the API returns, and the handler sees one create call. |
| Read | A second plan with the same config is empty. A no-op read must not call a write endpoint. |
| Import | Supported resources import and match config, ignoring attributes the API cannot return. Unsupported resources use import id `not-supported` and must error. |
| Drift | Remote drift, or a config change where the API has no read, produces a non-empty plan. |
| Destroy | The destroy call matches the resource: cancel, delete, clear, or no HTTP call. |

Drift that would send another message, replace a team member, migrate a subscriber again, or assign another license is `PlanOnly`. Group rename and group-user memo repair are applied, because those updates do not spend credits.

## Coverage by resource

| Resource | Create | Read | Import | Drift | Destroy | Secrets |
|---|---|---|---|---|---|---|
| `pushover_message` | `POST /messages.json`. Emergency priority 2 stores `receipt`. | No-op. Unchanged config does not send again. | Unsupported. Import must error. | Config text change plans a replacement. Plan only, so a second message is not sent. | `cancel_on_destroy` cancels the emergency receipt. | `api_token` |
| `pushover_receipt` | `GET /receipts/{id}.json` stores status. | Refresh of unchanged status plans nothing. | Import id is the receipt. `cancel_on_destroy` is ignored because import does not set it. | `last_delivered_at` moves from 10 to 20 and the plan stays empty. | Cancel once. | `api_token` |
| `pushover_group` | `POST /groups/{key}/rename.json`. | `GET` returns the stored name. Empty plan. | Import id is the group key. State matches. | Remote name `Drifted` plans an update. Apply renames back to `Ops`. | No HTTP call. Delete is a state drop. | none |
| `pushover_group_user` | `POST .../add_user.json` with memo `on call`. | `GET` group membership. Empty plan. | Import id is `group/user`. State matches, including memo. | Remote memo `drifted` plans an update. Apply re-adds the member. | `POST .../delete_user.json` once. | none |
| `pushover_team_user` | `POST /teams/add_user.json`, then `GET /teams.json` adopts `user_id`. | Empty plan while admin stays false. | Import id is `token/user_id`. `name` and `password` are ignored: Read does not adopt an unmanaged name, and the API returns no password. | Remote `administrator: true` plans a replacement. Plan only. | `POST /teams/remove_user.json` once. Revoke is not called when remove succeeds. | `id`, `password`, `token` |
| `pushover_subscription_subscriber` | `POST /subscriptions/migrate.json` stores `subscribed_user_key`. | `POST /users/validate.json`. Empty plan while the key is valid. | Unsupported. Import must error. | Validate error `user key is invalid` drops state and plans a create. Plan only, so migrate runs once. | No HTTP call. | `api_token` |
| `pushover_license_assignment` | `POST /licenses/assign.json` once. Id is `user:<key>/Android`. | Read does not call assign. Empty plan. | Unsupported. Import must error. | Config `os` change to `iOS` plans a replacement. Plan only, so assign runs once. | No HTTP call. | `api_token` |
| `pushover_glances` | `POST /glances.json` with `text=ok`. | No-op. Unchanged config does not post again. | Unsupported. Import must error. | Config text change plans an in-place update. Plan only. | Destroy posts the previously set `text` as an empty string. | `api_token` |
