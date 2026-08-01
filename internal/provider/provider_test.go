// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
"os"
"regexp"
"testing"

"github.com/hashicorp/terraform-plugin-framework/providerserver"
"github.com/hashicorp/terraform-plugin-go/tfprotov6"
"github.com/hashicorp/terraform-plugin-testing/helper/resource"

"github.com/Josh-Archer/terraform-provider-pushover/internal/provider"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The provider is configured via the api_token attribute
// or the PUSHOVER_API_TOKEN environment variable.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
"pushover": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// skipIfNoToken skips the test when PUSHOVER_API_TOKEN is not set.
// Acceptance tests that make real API calls must call this helper.
func skipIfNoToken(t *testing.T) {
t.Helper()
if os.Getenv("PUSHOVER_API_TOKEN") == "" {
t.Skip("PUSHOVER_API_TOKEN not set; skipping acceptance test")
}
}

// ----- Provider configuration -----

func TestProvider_ValidConfig(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
// A valid provider config with at least one resource so Configure is invoked.
Config: `
provider "pushover" { api_token = "test_token_abc123" }

resource "pushover_message" "probe" {
  user_key = "uABC"
  message  = "probe"
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

func TestProvider_MissingToken(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
// A resource is included so Configure is actually invoked during plan.
Config: `
provider "pushover" {}

resource "pushover_message" "probe" {
  user_key = "uABC"
  message  = "probe"
}`,
PlanOnly:    true,
ExpectError: regexp.MustCompile(`(?i)(missing api token|api_token|PUSHOVER_API_TOKEN)`),
},
},
})
}

// ----- Resource presence -----

func TestProvider_HasMessageResource(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "tok" }

resource "pushover_message" "probe" {
  user_key = "uABC"
  message  = "probe"
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

func TestProvider_HasGroupUserResource(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "tok" }

resource "pushover_group_user" "probe" {
  group_key = "gABC"
  user_key  = "uABC"
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

func TestProvider_HasReceiptResource(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "tok" }

resource "pushover_receipt" "probe" {
  receipt = "rcpt_abcdefghijklmnopqrstuvwxyz012345"
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

// ----- Data source presence (acceptance; skipped when no API token) -----

func TestProvider_HasSoundsDataSource(t *testing.T) {
skipIfNoToken(t)
resource.Test(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" {}
data "pushover_sounds" "all" {}`,
Check: resource.ComposeTestCheckFunc(
resource.TestCheckResourceAttrSet("data.pushover_sounds.all", "id"),
),
},
},
})
}

func TestProvider_HasValidateUserDataSource(t *testing.T) {
skipIfNoToken(t)
userKey := os.Getenv("PUSHOVER_USER_KEY")
if userKey == "" {
t.Skip("PUSHOVER_USER_KEY not set; skipping acceptance test")
}
resource.Test(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" {}

data "pushover_validate_user" "check" {
  user_key = "` + userKey + `"
}`,
Check: resource.ComposeTestCheckFunc(
resource.TestCheckResourceAttrSet("data.pushover_validate_user.check", "id"),
),
},
},
})
}
