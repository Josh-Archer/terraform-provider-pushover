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
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestLicenseAssignmentID(t *testing.T) {
	t.Parallel()

	if got := licenseAssignmentID("uKey", "", ""); got != "user:uKey" {
		t.Errorf("unexpected id %q", got)
	}
	if got := licenseAssignmentID("uKey", "", "iOS"); got != "user:uKey/iOS" {
		t.Errorf("unexpected id %q", got)
	}
	if got := licenseAssignmentID("", "ada@example.com", ""); got != "email:ada@example.com" {
		t.Errorf("unexpected id %q", got)
	}
	if got := licenseAssignmentID("", "ada@example.com", "Desktop"); got != "email:ada@example.com/Desktop" {
		t.Errorf("unexpected id %q", got)
	}
}

func licenseAssignmentSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	res := &LicenseAssignmentResource{}
	schemaResp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema errors: %v", schemaResp.Diagnostics)
	}
	return *schemaResp
}

func licenseAssignmentPlan(t *testing.T, schemaResp resource.SchemaResponse, userKey, email, osVal, token *string) tfsdk.Plan {
	t.Helper()
	ctx := context.Background()
	tfType := schemaResp.Schema.Type().TerraformType(ctx)

	strVal := func(p *string) tftypes.Value {
		if p == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *p)
	}

	return tfsdk.Plan{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"id":         tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"user_key":   strVal(userKey),
			"email":      strVal(email),
			"os":         strVal(osVal),
			"api_token":  strVal(token),
			"credits":    tftypes.NewValue(tftypes.Number, tftypes.UnknownValue),
			"request_id": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}),
	}
}

func licenseAssignmentState(t *testing.T, schemaResp resource.SchemaResponse, id string, userKey, email, osVal, token *string) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	tfType := schemaResp.Schema.Type().TerraformType(ctx)

	strVal := func(p *string) tftypes.Value {
		if p == nil {
			return tftypes.NewValue(tftypes.String, nil)
		}
		return tftypes.NewValue(tftypes.String, *p)
	}

	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"id":         tftypes.NewValue(tftypes.String, id),
			"user_key":   strVal(userKey),
			"email":      strVal(email),
			"os":         strVal(osVal),
			"api_token":  strVal(token),
			"credits":    tftypes.NewValue(tftypes.Number, 5),
			"request_id": tftypes.NewValue(tftypes.String, "assign-req"),
		}),
	}
}

func TestLicenseAssignmentResource_Create_SuccessByUser(t *testing.T) {
	t.Parallel()

	var gotPath, gotUser, gotEmail, gotOS, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			return
		}
		gotUser = r.FormValue("user")
		gotEmail = r.FormValue("email")
		gotOS = r.FormValue("os")
		gotToken = r.FormValue("token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"credits":5,"request":"assign-req-1"}`))
	}))
	defer srv.Close()

	schemaResp := licenseAssignmentSchema(t)
	ctx := context.Background()
	userKey := "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	osVal := "iOS"
	plan := licenseAssignmentPlan(t, schemaResp, &userKey, nil, &osVal, nil)

	res := &LicenseAssignmentResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	if gotPath != "/licenses/assign.json" {
		t.Errorf("unexpected path %s", gotPath)
	}
	if gotUser != userKey || gotEmail != "" || gotOS != "iOS" || gotToken != "app-token" {
		t.Errorf("unexpected form user=%q email=%q os=%q token=%q", gotUser, gotEmail, gotOS, gotToken)
	}

	var got LicenseAssignmentResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.ID.ValueString() != "user:uQiRzpo4DXghDmr9QzzfQu27cmVRsG/iOS" {
		t.Errorf("unexpected id %q", got.ID.ValueString())
	}
	if got.Credits.ValueInt64() != 5 {
		t.Errorf("unexpected credits %d", got.Credits.ValueInt64())
	}
	if got.RequestID.ValueString() != "assign-req-1" {
		t.Errorf("unexpected request_id %q", got.RequestID.ValueString())
	}
}

func TestLicenseAssignmentResource_Create_SuccessByEmail(t *testing.T) {
	t.Parallel()

	var gotEmail, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			return
		}
		gotEmail = r.FormValue("email")
		gotToken = r.FormValue("token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"credits":2,"request":"assign-req-email"}`))
	}))
	defer srv.Close()

	schemaResp := licenseAssignmentSchema(t)
	ctx := context.Background()
	email := "ada@example.com"
	token := "override-token"
	plan := licenseAssignmentPlan(t, schemaResp, nil, &email, nil, &token)

	res := &LicenseAssignmentResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	if gotEmail != "ada@example.com" || gotToken != "override-token" {
		t.Errorf("unexpected form email=%q token=%q", gotEmail, gotToken)
	}

	var got LicenseAssignmentResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.ID.ValueString() != "email:ada@example.com" {
		t.Errorf("unexpected id %q", got.ID.ValueString())
	}
	if got.Credits.ValueInt64() != 2 {
		t.Errorf("unexpected credits %d", got.Credits.ValueInt64())
	}
}

func TestLicenseAssignmentResource_Create_APIErrorLeavesStateUnset(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"request":"55f7df29-e2f3-4333-aeef-336334fafcee","errors":{"token":["is out of available license credits"]}}`))
	}))
	defer srv.Close()

	schemaResp := licenseAssignmentSchema(t)
	ctx := context.Background()
	userKey := "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	plan := licenseAssignmentPlan(t, schemaResp, &userKey, nil, nil, nil)

	res := &LicenseAssignmentResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected create error")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected no state after failed create")
	}
}

func TestLicenseAssignmentResource_Create_ValidationErrorsMakeNoNetworkCall(t *testing.T) {
	t.Parallel()

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	schemaResp := licenseAssignmentSchema(t)
	ctx := context.Background()
	res := &LicenseAssignmentResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}

	// Neither user_key nor email
	resp1 := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: licenseAssignmentPlan(t, schemaResp, nil, nil, nil, nil)}, resp1)
	if !resp1.Diagnostics.HasError() {
		t.Fatal("expected error when neither user_key nor email is set")
	}

	// Both user_key and email
	u := "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	e := "ada@example.com"
	resp2 := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: licenseAssignmentPlan(t, schemaResp, &u, &e, nil, nil)}, resp2)
	if !resp2.Diagnostics.HasError() {
		t.Fatal("expected error when both user_key and email are set")
	}

	// Invalid os
	badOS := "Linux"
	resp3 := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: licenseAssignmentPlan(t, schemaResp, &u, nil, &badOS, nil)}, resp3)
	if !resp3.Diagnostics.HasError() {
		t.Fatal("expected error for invalid os")
	}

	if called {
		t.Fatal("validation failures must not call the API")
	}
}

func TestLicenseAssignmentResource_Read_MakesNoNetworkCall(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request during Read: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	schemaResp := licenseAssignmentSchema(t)
	userKey := "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	state := licenseAssignmentState(t, schemaResp, "user:uQiRzpo4DXghDmr9QzzfQu27cmVRsG", &userKey, nil, nil, nil)

	res := &LicenseAssignmentResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to remain intact")
	}
}

func TestLicenseAssignmentResource_Delete_IsNoOpWithWarning(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request during Delete: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	schemaResp := licenseAssignmentSchema(t)
	userKey := "uQiRzpo4DXghDmr9QzzfQu27cmVRsG"
	state := licenseAssignmentState(t, schemaResp, "user:uQiRzpo4DXghDmr9QzzfQu27cmVRsG", &userKey, nil, nil, nil)

	res := &LicenseAssignmentResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.DeleteResponse{State: state}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() != 1 {
		t.Fatalf("expected 1 warning diagnostic on delete, got %d (%v)", resp.Diagnostics.WarningsCount(), resp.Diagnostics)
	}
	warning := resp.Diagnostics.Warnings()[0]
	if !strings.Contains(strings.ToLower(warning.Summary()+" "+warning.Detail()), "non-refundable") {
		t.Errorf("expected warning to mention non-refundable credits, got %q: %q", warning.Summary(), warning.Detail())
	}
}
