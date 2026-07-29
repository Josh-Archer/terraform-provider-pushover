// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
)

// TestReceiptResource_Schema validates the minimal required fields are accepted.
func TestReceiptResource_Schema(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake_token_for_schema_test" }

resource "pushover_receipt" "test" {
  receipt = "rcpt_abcdefghijklmnopqrstuvwxyz012345"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestReceiptResource_CancelOnDestroyOptional validates cancel_on_destroy is accepted.
func TestReceiptResource_CancelOnDestroyOptional(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_receipt" "no_cancel" {
  receipt           = "rcpt_abcdefghijklmnopqrstuvwxyz012345"
  cancel_on_destroy = false
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestReceiptResource_DefaultCancelOnDestroy validates the default plan value is true.
func TestReceiptResource_DefaultCancelOnDestroy(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_receipt" "defaults" {
  receipt = "rcpt_abcdefghijklmnopqrstuvwxyz012345"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(
						"pushover_receipt.defaults", "cancel_on_destroy", "true",
					),
				),
			},
		},
	})
}

// TestReceiptResource_WithEmergencyMessageExample validates a typical emergency lifecycle config.
func TestReceiptResource_WithEmergencyMessageExample(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_message" "outage" {
  user_key = "utest1234567890abcdefghijklmnopqr"
  message  = "Production database is DOWN!"
  title    = "CRITICAL OUTAGE"
  priority = 2
  retry    = 60
  expire   = 3600
}

# receipt is unknown until apply; for plan-only we pass a static receipt.
resource "pushover_receipt" "outage" {
  receipt           = "rcpt_static_for_plan_only_test_abcdef"
  cancel_on_destroy = true
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestReceiptResource_LifecycleWithMockClient exercises GetReceipt + CancelReceipt the way
// the pushover_receipt resource Create/Read/Delete paths use them.
func TestReceiptResource_LifecycleWithMockClient(t *testing.T) {
	t.Parallel()

	var cancelCalled bool
	var getCalls int
	var cancelPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/receipts/"):
			getCalls++
			if !strings.Contains(r.URL.RawQuery, "token=") {
				t.Errorf("expected token query param on GetReceipt, got %q", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":                 1,
				"request":                "req-receipt-1",
				"acknowledged":           0,
				"acknowledged_at":        0,
				"acknowledged_by":        "",
				"acknowledged_by_device": "",
				"last_delivered_at":      1700000010,
				"expired":                0,
				"expires_at":             1700003600,
				"called_back":            0,
				"called_back_at":         0,
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/cancel.json"):
			cancelCalled = true
			cancelPath = r.URL.Path
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}
			if r.FormValue("token") != "tok" {
				t.Errorf("unexpected token: %s", r.FormValue("token"))
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  1,
				"request": "req-cancel-1",
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":0,"errors":["not found"]}`))
		}
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())

	// Create/Read path: refresh status from receipt API.
	got, err := client.GetReceipt(context.Background(), "rcpt_abc")
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	if got.Acknowledged != 0 {
		t.Errorf("expected not acknowledged, got %d", got.Acknowledged)
	}
	if got.ExpiresAt != 1700003600 {
		t.Errorf("unexpected expires_at: %d", got.ExpiresAt)
	}
	if got.Request != "req-receipt-1" {
		t.Errorf("unexpected request id: %s", got.Request)
	}

	// Delete path (cancel_on_destroy = true, not yet acknowledged/expired).
	cancelResp, err := client.CancelReceipt(context.Background(), "rcpt_abc")
	if err != nil {
		t.Fatalf("CancelReceipt: %v", err)
	}
	if cancelResp.Status != 1 {
		t.Errorf("expected status 1, got %d", cancelResp.Status)
	}
	if !cancelCalled {
		t.Error("expected cancel endpoint to be called")
	}
	if !strings.Contains(cancelPath, "rcpt_abc") {
		t.Errorf("expected receipt id in cancel path, got %q", cancelPath)
	}
	if getCalls < 1 {
		t.Error("expected GetReceipt to be called")
	}
}

// TestReceiptResource_AcknowledgedStatusWithMockClient verifies acknowledged emergency status mapping.
func TestReceiptResource_AcknowledgedStatusWithMockClient(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":                 1,
			"request":                "req-2",
			"acknowledged":           1,
			"acknowledged_at":        1700001000,
			"acknowledged_by":        "uABC",
			"acknowledged_by_device": "iphone",
			"last_delivered_at":      1700000010,
			"expired":                0,
			"expires_at":             1700003600,
			"called_back":            1,
			"called_back_at":         1700001001,
		})
	}))
	defer srv.Close()

	client := pushover.NewClientWithBase("tok", srv.URL, srv.Client())
	result, err := client.GetReceipt(context.Background(), "rcpt_acked")
	if err != nil {
		t.Fatalf("GetReceipt: %v", err)
	}
	if result.Acknowledged != 1 {
		t.Fatalf("expected acknowledged=1")
	}
	if result.AcknowledgedBy != "uABC" {
		t.Errorf("unexpected acknowledged_by: %s", result.AcknowledgedBy)
	}
	if result.AcknowledgedByDevice != "iphone" {
		t.Errorf("unexpected acknowledged_by_device: %s", result.AcknowledgedByDevice)
	}
	if result.CalledBack != 1 {
		t.Errorf("expected called_back=1")
	}
	// Resource Delete skips cancel when already acknowledged.
}
