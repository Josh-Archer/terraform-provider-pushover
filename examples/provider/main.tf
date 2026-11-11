# Configure the Pushover provider.
# The api_token can also be provided via the PUSHOVER_API_TOKEN environment variable.
#
# The provider is published on the OpenTofu registry (registry.opentofu.org).
# That explicit hostname installs under both OpenTofu and Terraform.
# It is not yet published on registry.terraform.io (see issue #51), so the
# short "josh-archer/pushover" source only resolves under OpenTofu.

terraform {
  required_providers {
    pushover = {
      source  = "registry.opentofu.org/josh-archer/pushover"
      version = "~> 0.0.3"
    }
  }
}

provider "pushover" {
  # Use a variable or environment variable – never hardcode credentials.
  api_token = var.pushover_api_token
}

variable "pushover_api_token" {
  description = "Pushover application API token."
  type        = string
  sensitive   = true
}
