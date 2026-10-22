// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestSubscriptionSubscriberResource_Schema(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake_token_for_schema_test" }

resource "pushover_subscription_subscriber" "alice" {
  subscription = "alerts-a1b2c3"
  user_key     = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestSubscriptionSubscriberResource_AllOptionalFields(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_subscription_subscriber" "alice" {
  subscription = "alerts-a1b2c3"
  user_key     = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  device_name  = "iphone"
  sound        = "pushover"
  api_token    = "override-token"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestSubscriptionSubscriberResource_DeviceNameInvalid(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_subscription_subscriber" "alice" {
  subscription = "alerts-a1b2c3"
  user_key     = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  device_name  = "not a device"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`device name must be 1-25 characters`),
			},
		},
	})
}

func TestSubscriptionSubscriberResource_ApplyNoOpAndReplace(t *testing.T) {
	var migrateCalls int32
	var mu sync.Mutex
	var sounds []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/subscriptions/migrate.json":
			atomic.AddInt32(&migrateCalls, 1)
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				return
			}
			mu.Lock()
			sounds = append(sounds, r.FormValue("sound"))
			mu.Unlock()
			if r.FormValue("token") != "fake_token" {
				t.Errorf("unexpected token %q", r.FormValue("token"))
			}
			if r.FormValue("subscription") != "alerts-a1b2c3" {
				t.Errorf("unexpected subscription %q", r.FormValue("subscription"))
			}
			if r.FormValue("user") != "uQiRzpo4DXghDmr9QzzfQu27cmVRsG" {
				t.Errorf("unexpected user %q", r.FormValue("user"))
			}
			_, _ = w.Write([]byte(`{"status":1,"request":"migrate-req-1","subscribed_user_key":"sPfjsD2fGzEd9TR52DU31Hv4A61Vvk"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/users/validate.json":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				return
			}
			if r.FormValue("user") != "sPfjsD2fGzEd9TR52DU31Hv4A61Vvk" {
				t.Errorf("validated unexpected user %q", r.FormValue("user"))
			}
			_, _ = w.Write([]byte(`{"status":1,"request":"validate-req","group":0,"devices":[],"licenses":[]}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":0,"errors":["not found"]}`))
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	base := `
provider "pushover" { api_token = "fake_token" }

resource "pushover_subscription_subscriber" "alice" {
  subscription = "alerts-a1b2c3"
  user_key     = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  device_name  = "iphone"
  sound        = "pushover"
}
`
	replaced := `
provider "pushover" { api_token = "fake_token" }

resource "pushover_subscription_subscriber" "alice" {
  subscription = "alerts-a1b2c3"
  user_key     = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  device_name  = "iphone"
  sound        = "bike"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: base,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_subscription_subscriber.alice", "id", "alerts-a1b2c3/uQiRzpo4DXghDmr9QzzfQu27cmVRsG/iphone"),
					resource.TestCheckResourceAttr("pushover_subscription_subscriber.alice", "subscribed_user_key", "sPfjsD2fGzEd9TR52DU31Hv4A61Vvk"),
					resource.TestCheckResourceAttr("pushover_subscription_subscriber.alice", "request_id", "migrate-req-1"),
					resource.TestCheckResourceAttr("pushover_subscription_subscriber.alice", "sound", "pushover"),
				),
			},
			{
				Config:             base,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				// sound forces replacement, which migrates again. An in-place update
				// would change sound without calling the migration API.
				Config: replaced,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_subscription_subscriber.alice", "sound", "bike"),
					resource.TestCheckResourceAttr("pushover_subscription_subscriber.alice", "subscribed_user_key", "sPfjsD2fGzEd9TR52DU31Hv4A61Vvk"),
					resource.TestCheckResourceAttr("pushover_subscription_subscriber.alice", "request_id", "migrate-req-1"),
				),
			},
		},
	})

	if got := atomic.LoadInt32(&migrateCalls); got != 2 {
		t.Fatalf("expected migration on create and on sound replacement, got %d calls", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sounds) != 2 || sounds[0] != "pushover" || sounds[1] != "bike" {
		t.Fatalf("expected sounds [pushover bike], got %v", sounds)
	}
}
