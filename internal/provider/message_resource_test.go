// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
"regexp"
"testing"

"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestMessageResource_Schema validates the minimal required fields are accepted.
func TestMessageResource_Schema(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake_token_for_schema_test" }

resource "pushover_message" "test" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "Hello from Terraform!"
  title    = "Test"
  priority = 0
}`,
// PlanOnly so we validate schema without hitting the real API.
// ExpectNonEmptyPlan because the resource doesn't exist yet.
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

// TestMessageResource_AllOptionalFields ensures all optional fields are accepted.
func TestMessageResource_AllOptionalFields(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "full" {
  user_key   = "utest1234567890abcdefghijklmnopqr"
  message    = "Detailed message"
  title      = "Detailed Title"
  url        = "https://example.com"
  url_title  = "Click"
  priority   = 1
  sound      = "bike"
  device     = "iphone"
  html       = true
  monospace  = false
  ttl        = 3600
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

// TestMessageResource_EmergencyPriorityFields validates emergency fields are accepted.
func TestMessageResource_EmergencyPriorityFields(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "emergency" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "Emergency!"
  priority = 2
  retry    = 60
  expire   = 3600
  callback = "https://example.com/ack"
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

// TestMessageResource_LowPriority validates negative priority values are accepted.
func TestMessageResource_LowPriority(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "low" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "quiet notification"
  priority = -2
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

// ----- Validation error tests -----

// TestMessageResource_PriorityOutOfRange expects a validation error for priority > 2.
func TestMessageResource_PriorityOutOfRange(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "bad" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "Bad priority"
  priority = 5
}`,
PlanOnly:    true,
ExpectError: regexp.MustCompile(`(?i)(value must be between|invalid)`),
},
},
})
}

// TestMessageResource_MessageTooLong expects a validation error for a message > 1024 chars.
func TestMessageResource_MessageTooLong(t *testing.T) {
t.Parallel()

longMsg := make([]byte, 1025)
for i := range longMsg {
longMsg[i] = 'a'
}

resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "long" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "` + string(longMsg) + `"
}`,
PlanOnly:    true,
ExpectError: regexp.MustCompile(`(?i)(length|characters)`),
},
},
})
}

// TestMessageResource_NegativeTTL expects a validation error for ttl < 1.
func TestMessageResource_NegativeTTL(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "neg_ttl" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "test"
  ttl      = -1
}`,
PlanOnly:    true,
ExpectError: regexp.MustCompile(`(?i)(value must be at least|invalid)`),
},
},
})
}

// TestMessageResource_RetryBelowMinimum expects a validation error for retry < 30.
func TestMessageResource_RetryBelowMinimum(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "low_retry" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "test"
  priority = 2
  retry    = 10
  expire   = 3600
}`,
PlanOnly:    true,
ExpectError: regexp.MustCompile(`(?i)(value must be at least|invalid)`),
},
},
})
}

// TestMessageResource_ExpireExceedsMaximum expects a validation error for expire > 10800.
func TestMessageResource_ExpireExceedsMaximum(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "big_expire" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "test"
  priority = 2
  retry    = 30
  expire   = 99999
}`,
PlanOnly:    true,
ExpectError: regexp.MustCompile(`(?i)(value must be between|invalid)`),
},
},
})
}

// TestMessageResource_TitleTooLong expects a validation error for title > 250 chars.
func TestMessageResource_TitleTooLong(t *testing.T) {
t.Parallel()

longTitle := make([]byte, 251)
for i := range longTitle {
longTitle[i] = 'T'
}

resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "long_title" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "test"
  title    = "` + string(longTitle) + `"
}`,
PlanOnly:    true,
ExpectError: regexp.MustCompile(`(?i)(length|characters)`),
},
},
})
}

// TestMessageResource_AttachmentFieldsAccepted ensures attachment attributes are in the schema.
func TestMessageResource_AttachmentFieldsAccepted(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "with_attach" {
  user_key        = "utest1234567890abcdefghijklmnopqr"
  message         = "see image"
  attachment      = "/tmp/screenshot.png"
  attachment_type = "image/png"
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}

// TestMessageResource_RemoteAttachmentURLAccepted ensures http(s) attachment URLs are accepted.
func TestMessageResource_RemoteAttachmentURLAccepted(t *testing.T) {
t.Parallel()
resource.UnitTest(t, resource.TestCase{
ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
Steps: []resource.TestStep{
{
Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "with_url_attach" {
  user_key   = "utest1234567890abcdefghijklmnopqr"
  message    = "remote image"
  attachment = "https://example.com/status/graph.png"
}`,
PlanOnly:           true,
ExpectNonEmptyPlan: true,
},
},
})
}
