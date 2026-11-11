---
page_title: "Pushover Provider"
description: |-
  Use the Pushover provider to send push notifications, update glance widgets, manage delivery groups and team membership, migrate users onto application subscriptions, and manage pre-paid license credits via the Pushover API.
---

# Pushover Provider

The **Pushover** provider integrates with the [Pushover](https://pushover.net) push notification service. It allows you to send notifications, manage delivery group membership, manage Pushover team members, migrate users onto application subscriptions, and query or assign pre-paid license credits directly from Terraform or OpenTofu.

## Example Usage

The provider is published on the [OpenTofu registry](https://registry.opentofu.org/providers/josh-archer/pushover/latest)
(latest: `v0.0.3`). It is not yet published on `registry.terraform.io`
(manual namespace step, see [issue #51](https://github.com/Josh-Archer/terraform-provider-pushover/issues/51)),
so Terraform configurations must use the explicit OpenTofu registry hostname.

```terraform
# Works under both OpenTofu and Terraform.
terraform {
  required_providers {
    pushover = {
      source  = "registry.opentofu.org/josh-archer/pushover"
      version = "~> 0.0.3"
    }
  }
}

provider "pushover" {
  api_token = var.pushover_api_token
}
```

OpenTofu also resolves the short source `josh-archer/pushover` (same version
constraint) via its default registry. For local builds, see the
`dev_overrides` path in the README Quick Start.

The `api_token` can also be supplied via the `PUSHOVER_API_TOKEN` environment variable, which is the recommended approach for CI/CD pipelines:

```bash
export PUSHOVER_API_TOKEN="your_application_token"
terraform apply
```

## Authentication

You will need a **Pushover application API token**. Create one by registering an application at [https://pushover.net/apps/build](https://pushover.net/apps/build).

Team membership (`pushover_team_user`) uses a team API token on the resource. Create that token in the team settings at [https://pushover.net/teams](https://pushover.net/teams).

## Reliability

The provider's HTTP client automatically retries **transient** Pushover API failures:

- HTTP **429** (rate limited)
- HTTP **5xx** (server errors)

Retries are **bounded** (up to 3 retries after the initial attempt) with **exponential backoff** (starting at 500ms, capped at 8s). When the response includes a `Retry-After` header, that value is honored instead of the default backoff.

Non-retryable client errors (for example HTTP 4xx validation failures) fail immediately without retry.

## Schema

### Required (one of)

- `api_token` (String, Sensitive) — Pushover application API token. Can also be provided via the `PUSHOVER_API_TOKEN` environment variable.

## Resources

- [pushover_message](resources/message.md) — Send a push notification.
- [pushover_group](resources/group.md) — Manage the name of an existing Pushover delivery group.
- [pushover_group_user](resources/group_user.md) — Add a user to a Pushover delivery group.
- [pushover_team_user](resources/team_user.md) — Add or remove a user on a Pushover team.
- [pushover_subscription_subscriber](resources/subscription_subscriber.md) — Migrate a user onto an application subscription.
- [pushover_license_assignment](resources/license_assignment.md) — Assign a pre-paid license credit to a user or email.
- [pushover_glances](resources/glances.md) — Update a smartwatch/lock-screen glance widget.

## Data Sources

- [pushover_sounds](data-sources/sounds.md) — List available notification sounds.
- [pushover_validate_user](data-sources/validate_user.md) — Validate a user or group key.
- [pushover_limits](data-sources/limits.md) — Query monthly message quota and remaining limit.
- [pushover_licenses](data-sources/licenses.md) — Query remaining pre-paid license credits.
