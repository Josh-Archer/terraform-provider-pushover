// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestTeamUserResource_BasicSchema validates the minimal required fields are accepted.
func TestTeamUserResource_BasicSchema(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "basic" {
  token = "team-token"
  email = "ada@example.com"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestTeamUserResource_AllOptionalFields validates the add-user arguments are accepted.
func TestTeamUserResource_AllOptionalFields(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "full" {
  token    = "team-token"
  email    = "ada@example.com"
  name     = "Ada Lovelace"
  password = "not-a-real-password"
  instant  = true
  admin    = true
  group    = "Ops"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestTeamUserResource_Defaults validates admin and instant default to false.
func TestTeamUserResource_Defaults(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "defaults" {
  token = "team-token"
  email = "ada@example.com"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.defaults", "admin", "false"),
					resource.TestCheckResourceAttr("pushover_team_user.defaults", "instant", "false"),
				),
			},
		},
	})
}

// TestTeamUserResource_RejectsBadEmail validates the email format check.
func TestTeamUserResource_RejectsBadEmail(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "bad" {
  token = "team-token"
  email = "not-an-email"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must be an email address`),
			},
		},
	})
}
