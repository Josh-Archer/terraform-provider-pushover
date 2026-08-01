// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
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

// TestMessageResource_IdempotencyKeySchema accepts the optional idempotency_key attribute.
func TestMessageResource_IdempotencyKeySchema(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "keyed" {
  user_key         = "utest1234567890abcdefghijklmnopqr"
  message          = "Deploy finished"
  title            = "Deploy"
  idempotency_key  = "release-1.2.3"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestMessageResource_IdempotencyKeyTooLong expects a validation error for keys > 256 chars.
func TestMessageResource_IdempotencyKeyTooLong(t *testing.T) {
	t.Parallel()

	longKey := make([]byte, 257)
	for i := range longKey {
		longKey[i] = 'k'
	}

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "long_key" {
  user_key        = "utest1234567890abcdefghijklmnopqr"
  message         = "test"
  idempotency_key = "` + string(longKey) + `"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?i)(length|characters)`),
			},
		},
	})
}

// TestMessageResource_OneShotLifecycleSchema validates that lifecycle.ignore_changes
// is accepted for the recommended one-shot pattern (no apply; schema/plan only).
func TestMessageResource_OneShotLifecycleSchema(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "oneshot" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "Bootstrap complete"
  title    = "One-shot"

  lifecycle {
    ignore_changes = all
  }
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// mockPushoverMessages starts an httptest that answers POST /messages.json successfully
// and records how many times it was called.
func mockPushoverMessages(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var sends int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages.json") {
			atomic.AddInt32(&sends, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":1,"request":"mock-request-id","receipt":""}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":0,"errors":["not found"]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &sends
}

// TestMessageResource_NoOpPlanWhenUnchanged applies once, then expects an empty plan
// when the configuration is unchanged (no accidental re-send).
func TestMessageResource_NoOpPlanWhenUnchanged(t *testing.T) {
	srv, sends := mockPushoverMessages(t)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	cfg := `
provider "pushover" { api_token = "fake_token" }

resource "pushover_message" "stable" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "stable message"
  title    = "Stable"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_message.stable", "request_id", "mock-request-id"),
					resource.TestCheckResourceAttr("pushover_message.stable", "message", "stable message"),
				),
			},
			{
				// Second plan with identical config must be a no-op (no re-send).
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})

	if got := atomic.LoadInt32(sends); got != 1 {
		t.Fatalf("expected exactly 1 send, got %d", got)
	}
}

// TestMessageResource_OneShotIgnoreChangesNoOp applies a one-shot message, then changes
// content attributes while lifecycle.ignore_changes = all is set and expects a no-op plan
// (no re-send on unrelated content churn).
func TestMessageResource_OneShotIgnoreChangesNoOp(t *testing.T) {
	srv, sends := mockPushoverMessages(t)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake_token" }

resource "pushover_message" "oneshot" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "first send"
  title    = "First"
  priority = 0

  lifecycle {
    ignore_changes = all
  }
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_message.oneshot", "message", "first send"),
					resource.TestCheckResourceAttr("pushover_message.oneshot", "request_id", "mock-request-id"),
				),
			},
			{
				// Content churn must not produce a plan when ignore_changes = all.
				Config: `
provider "pushover" { api_token = "fake_token" }

resource "pushover_message" "oneshot" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "changed body that would otherwise re-send"
  title    = "Changed Title"
  priority = 1
  sound    = "magic"

  lifecycle {
    ignore_changes = all
  }
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})

	if got := atomic.LoadInt32(sends); got != 1 {
		t.Fatalf("expected exactly 1 send after content churn with ignore_changes, got %d", got)
	}
}

// TestMessageResource_IdempotencyKeyControlsReplace verifies that with ignore_changes on
// content fields, only an idempotency_key change produces a non-empty (replace) plan.
func TestMessageResource_IdempotencyKeyControlsReplace(t *testing.T) {
	srv, sends := mockPushoverMessages(t)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake_token" }

resource "pushover_message" "versioned" {
  user_key        = "utest1234567890abcdefghijklmnopqr"
  message         = "Deploy finished"
  title           = "Deploy"
  idempotency_key = "v1.0.0"

  lifecycle {
    ignore_changes = [
      message,
      title,
      priority,
      sound,
      device,
      url,
      url_title,
      html,
      monospace,
      ttl,
      timestamp,
      retry,
      expire,
      callback,
      api_token,
      user_key,
    ]
  }
}`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_message.versioned", "idempotency_key", "v1.0.0"),
					resource.TestCheckResourceAttr("pushover_message.versioned", "request_id", "mock-request-id"),
				),
			},
			{
				// Content changes alone must be ignored.
				Config: `
provider "pushover" { api_token = "fake_token" }

resource "pushover_message" "versioned" {
  user_key        = "utest1234567890abcdefghijklmnopqr"
  message         = "Deploy finished (edited copy)"
  title           = "Deploy (edited)"
  idempotency_key = "v1.0.0"

  lifecycle {
    ignore_changes = [
      message,
      title,
      priority,
      sound,
      device,
      url,
      url_title,
      html,
      monospace,
      ttl,
      timestamp,
      retry,
      expire,
      callback,
      api_token,
      user_key,
    ]
  }
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				// Changing the idempotency key must force replacement (non-empty plan).
				Config: `
provider "pushover" { api_token = "fake_token" }

resource "pushover_message" "versioned" {
  user_key        = "utest1234567890abcdefghijklmnopqr"
  message         = "Deploy finished (edited copy)"
  title           = "Deploy (edited)"
  idempotency_key = "v1.1.0"

  lifecycle {
    ignore_changes = [
      message,
      title,
      priority,
      sound,
      device,
      url,
      url_title,
      html,
      monospace,
      ttl,
      timestamp,
      retry,
      expire,
      callback,
      api_token,
      user_key,
    ]
  }
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})

	// Only the initial apply should have sent; the last step is plan-only.
	if got := atomic.LoadInt32(sends); got != 1 {
		t.Fatalf("expected exactly 1 send (plan-only key change), got %d", got)
	}
}
