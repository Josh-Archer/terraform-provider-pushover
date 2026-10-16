// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func createMessageTestState(t *testing.T, res *MessageResource, model MessageResourceModel) tfsdk.State {
	t.Helper()
	schemaResp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema error: %v", schemaResp.Diagnostics)
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(context.Background(), &model)
	if diags.HasError() {
		t.Fatalf("state.Set error: %v", diags)
	}
	return state
}

func TestMessageResource_Delete_CancelsEmergencyReceipt(t *testing.T) {
	t.Parallel()

	var cancelCalled bool
	var cancelPath string
	var cancelToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/cancel.json") {
			cancelCalled = true
			cancelPath = r.URL.Path
			_ = r.ParseForm()
			cancelToken = r.FormValue("token")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":1,"request":"req-cancel"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":0,"errors":["not found"]}`))
	}))
	defer srv.Close()

	res := &MessageResource{
		client: pushover.NewClientWithBase("provider_token", srv.URL, srv.Client()),
	}

	state := createMessageTestState(t, res, MessageResourceModel{
		UserKey:   types.StringValue("utest123"),
		Message:   types.StringValue("Emergency alert"),
		Priority:  types.Int64Value(2),
		Retry:     types.Int64Value(60),
		Expire:    types.Int64Value(3600),
		Receipt:   types.StringValue("rcpt_test_emergency"),
		RequestID: types.StringValue("req-1"),
	})

	resp := &resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !cancelCalled {
		t.Fatal("expected CancelReceiptWithToken to be called on Delete")
	}
	if !strings.Contains(cancelPath, "rcpt_test_emergency") {
		t.Errorf("expected receipt in cancel path, got %q", cancelPath)
	}
	if cancelToken != "provider_token" {
		t.Errorf("expected token %q, got %q", "provider_token", cancelToken)
	}
}

func TestMessageResource_Delete_WithAPITokenOverride(t *testing.T) {
	t.Parallel()

	var cancelCalled bool
	var cancelToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/cancel.json") {
			cancelCalled = true
			_ = r.ParseForm()
			cancelToken = r.FormValue("token")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":1,"request":"req-cancel"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":0,"errors":["not found"]}`))
	}))
	defer srv.Close()

	res := &MessageResource{
		client: pushover.NewClientWithBase("default_token", srv.URL, srv.Client()),
	}

	state := createMessageTestState(t, res, MessageResourceModel{
		UserKey:   types.StringValue("utest123"),
		Message:   types.StringValue("Emergency alert"),
		Priority:  types.Int64Value(2),
		Retry:     types.Int64Value(60),
		Expire:    types.Int64Value(3600),
		APIToken:  types.StringValue("override_token"),
		Receipt:   types.StringValue("rcpt_override"),
		RequestID: types.StringValue("req-1"),
	})

	resp := &resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !cancelCalled {
		t.Fatal("expected CancelReceiptWithToken to be called on Delete")
	}
	if cancelToken != "override_token" {
		t.Errorf("expected override token %q, got %q", "override_token", cancelToken)
	}
}

func TestMessageResource_Delete_NonEmergencyMessage_NoOp(t *testing.T) {
	t.Parallel()

	var cancelCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/cancel.json") {
			cancelCalled = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := &MessageResource{
		client: pushover.NewClientWithBase("provider_token", srv.URL, srv.Client()),
	}

	testCases := []struct {
		name     string
		priority types.Int64
		receipt  types.String
	}{
		{
			name:     "priority 0 null receipt",
			priority: types.Int64Value(0),
			receipt:  types.StringNull(),
		},
		{
			name:     "priority 0 empty receipt",
			priority: types.Int64Value(0),
			receipt:  types.StringValue(""),
		},
		{
			name:     "priority 0 non-empty receipt",
			priority: types.Int64Value(0),
			receipt:  types.StringValue("rcpt_priority_0"),
		},
		{
			name:     "priority 1 non-empty receipt",
			priority: types.Int64Value(1),
			receipt:  types.StringValue("rcpt_priority_1"),
		},
		{
			name:     "null priority non-empty receipt",
			priority: types.Int64Null(),
			receipt:  types.StringValue("rcpt_null_priority"),
		},
		{
			name:     "priority -1 non-empty receipt",
			priority: types.Int64Value(-1),
			receipt:  types.StringValue("rcpt_priority_neg1"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cancelCalled = false
			state := createMessageTestState(t, res, MessageResourceModel{
				UserKey:   types.StringValue("utest123"),
				Message:   types.StringValue("Normal message"),
				Priority:  tc.priority,
				Receipt:   tc.receipt,
				RequestID: types.StringValue("req-1"),
			})

			resp := &resource.DeleteResponse{}
			res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
			if cancelCalled {
				t.Fatalf("expected CancelReceiptWithToken not to be called for %s", tc.name)
			}
		})
	}
}

func TestMessageResource_Delete_AlreadyResolvedReceiptSucceeds(t *testing.T) {
	t.Parallel()

	errorResponses := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "already cancelled",
			statusCode: http.StatusOK,
			body:       `{"status":0,"errors":["already cancelled"]}`,
		},
		{
			name:       "already canceled single l",
			statusCode: http.StatusOK,
			body:       `{"status":0,"errors":["already canceled"]}`,
		},
		{
			name:       "already acknowledged",
			statusCode: http.StatusOK,
			body:       `{"status":0,"errors":["already acknowledged"]}`,
		},
		{
			name:       "receipt expired",
			statusCode: http.StatusOK,
			body:       `{"status":0,"errors":["expired"]}`,
		},
		{
			name:       "receipt not found",
			statusCode: http.StatusNotFound,
			body:       `{"status":0,"errors":["receipt not found"]}`,
		},
	}

	for _, tc := range errorResponses {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			res := &MessageResource{
				client: pushover.NewClientWithBase("provider_token", srv.URL, srv.Client()),
			}

			state := createMessageTestState(t, res, MessageResourceModel{
				UserKey:   types.StringValue("utest123"),
				Message:   types.StringValue("Emergency alert"),
				Priority:  types.Int64Value(2),
				Retry:     types.Int64Value(60),
				Expire:    types.Int64Value(3600),
				Receipt:   types.StringValue("rcpt_resolved"),
				RequestID: types.StringValue("req-1"),
			})

			resp := &resource.DeleteResponse{}
			res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

			if resp.Diagnostics.HasError() {
				t.Fatalf("expected already-resolved receipt to succeed, got: %v", resp.Diagnostics)
			}
		})
	}
}

func TestMessageResource_Delete_APIError_ReportsDiagnostic(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"errors":["application token is invalid"]}`))
	}))
	defer srv.Close()

	res := &MessageResource{
		client: pushover.NewClientWithBase("provider_token", srv.URL, srv.Client()),
	}

	state := createMessageTestState(t, res, MessageResourceModel{
		UserKey:   types.StringValue("utest123"),
		Message:   types.StringValue("Emergency alert"),
		Priority:  types.Int64Value(2),
		Retry:     types.Int64Value(60),
		Expire:    types.Int64Value(3600),
		Receipt:   types.StringValue("rcpt_err"),
		RequestID: types.StringValue("req-1"),
	})

	resp := &resource.DeleteResponse{}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("expected diagnostics error on cancel API failure, got none")
	}
	summary := resp.Diagnostics.Errors()[0].Summary()
	if summary != "Failed to cancel Pushover emergency receipt" {
		t.Errorf("unexpected error summary: %q", summary)
	}
}
