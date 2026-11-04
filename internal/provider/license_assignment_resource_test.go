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

func TestLicenseAssignmentResource_PlanMakesNoNetworkCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected network call during plan: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake_token" }

resource "pushover_license_assignment" "by_user" {
  user_key = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  os       = "iOS"
}

resource "pushover_license_assignment" "by_email" {
  email     = "ada@example.com"
  api_token = "override_token"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestLicenseAssignmentResource_ValidationErrors(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_license_assignment" "neither" {
  os = "Android"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?i)(user_key|email)`),
			},
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_license_assignment" "both" {
  user_key = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  email    = "ada@example.com"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?i)(user_key|email)`),
			},
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_license_assignment" "bad_os" {
  user_key = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  os       = "Windows"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?i)(Android|iOS|Desktop)`),
			},
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_license_assignment" "bad_email" {
  email = "not-an-email"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must be an email address`),
			},
		},
	})
}

func TestLicenseAssignmentResource_ApplyRefreshAndReplace(t *testing.T) {
	var totalCalls int32
	var mu sync.Mutex
	var osValues []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&totalCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/licenses/assign.json" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":0,"errors":["not found"]}`))
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			return
		}
		if r.FormValue("token") != "fake_token" {
			t.Errorf("unexpected token %q", r.FormValue("token"))
		}
		if r.FormValue("user") != "uQiRzpo4DXghDmr9QzzfQu27cmVRsG" {
			t.Errorf("unexpected user %q", r.FormValue("user"))
		}
		mu.Lock()
		osValues = append(osValues, r.FormValue("os"))
		mu.Unlock()

		if n == 1 {
			_, _ = w.Write([]byte(`{"status":1,"credits":5,"request":"assign-req-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":1,"credits":4,"request":"assign-req-2"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	base := `
provider "pushover" { api_token = "fake_token" }

resource "pushover_license_assignment" "alice" {
  user_key = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  os       = "iOS"
}
`
	replaced := `
provider "pushover" { api_token = "fake_token" }

resource "pushover_license_assignment" "alice" {
  user_key = "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
  os       = "Desktop"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: base,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "id", "user:uQiRzpo4DXghDmr9QzzfQu27cmVRsG/iOS"),
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "credits", "5"),
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "request_id", "assign-req-1"),
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "os", "iOS"),
				),
			},
			{
				// Refresh + plan on an existing assignment must make zero network calls.
				Config:             base,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				Config: replaced,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "id", "user:uQiRzpo4DXghDmr9QzzfQu27cmVRsG/Desktop"),
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "credits", "4"),
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "request_id", "assign-req-2"),
					resource.TestCheckResourceAttr("pushover_license_assignment.alice", "os", "Desktop"),
				),
			},
		},
	})

	if got := atomic.LoadInt32(&totalCalls); got != 2 {
		t.Fatalf("expected exactly 2 API calls (initial create + replacement create, 0 on refresh/destroy), got %d", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(osValues) != 2 || osValues[0] != "iOS" || osValues[1] != "Desktop" {
		t.Fatalf("expected os values [iOS Desktop], got %v", osValues)
	}
}

func TestLicensesDataSource_MockHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/licenses.json" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("token") != "fake_token" {
			t.Errorf("unexpected token %q", r.URL.Query().Get("token"))
		}
		_, _ = w.Write([]byte(`{"status":1,"credits":17,"request":"lic-ds-req-1"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake_token" }

data "pushover_licenses" "current" {}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.pushover_licenses.current", "id", "licenses"),
					resource.TestCheckResourceAttr("data.pushover_licenses.current", "credits", "17"),
					resource.TestCheckResourceAttr("data.pushover_licenses.current", "request_id", "lic-ds-req-1"),
				),
			},
		},
	})
}
