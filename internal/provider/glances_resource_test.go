// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestGlancesResource_Schema validates the minimal required fields are accepted.
func TestGlancesResource_Schema(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake_token_for_schema_test" }

resource "pushover_glances" "test" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  text     = "Garage door open"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestGlancesResource_AllFields ensures all glance data fields are accepted.
func TestGlancesResource_AllFields(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_glances" "full" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  device   = "iphone"
  title    = "Widgets Sold"
  text     = "42 today"
  subtext  = "Goal: 100"
  badge_count = 42
  percent     = 42
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestGlancesResource_NegativeCount allows negative badge_count values.
func TestGlancesResource_NegativeCount(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_glances" "neg" {
  user_key    = "utest1234567890abcdefghijklmnopqr"
  badge_count = -5
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestGlancesResource_PercentOutOfRange expects a validation error for percent > 100.
func TestGlancesResource_PercentOutOfRange(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_glances" "bad" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  percent  = 150
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?i)(value must be between|invalid)`),
			},
		},
	})
}

// TestGlancesResource_TitleTooLong expects a validation error for title > 100 chars.
func TestGlancesResource_TitleTooLong(t *testing.T) {
	t.Parallel()

	longTitle := make([]byte, 101)
	for i := range longTitle {
		longTitle[i] = 'T'
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_glances" "long" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  title    = "` + string(longTitle) + `"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?i)(length|characters)`),
			},
		},
	})
}

// TestGlancesResource_TextTooLong expects a validation error for text > 100 chars.
func TestGlancesResource_TextTooLong(t *testing.T) {
	t.Parallel()

	longText := make([]byte, 101)
	for i := range longText {
		longText[i] = 'x'
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_glances" "long" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  text     = "` + string(longText) + `"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?i)(length|characters)`),
			},
		},
	})
}
