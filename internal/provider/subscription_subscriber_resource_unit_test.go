// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestSubscriptionSubscriberID(t *testing.T) {
	t.Parallel()

	if got := subscriptionSubscriberID("alerts", "uKey", ""); got != "alerts/uKey" {
		t.Errorf("unexpected id %q", got)
	}
	if got := subscriptionSubscriberID("alerts", "uKey", "iphone"); got != "alerts/uKey/iphone" {
		t.Errorf("unexpected id %q", got)
	}
}

func TestIsSubscribedUserGoneError(t *testing.T) {
	t.Parallel()

	gone := []string{
		"pushover API error: user key is invalid",
		"pushover API error: user key is not valid",
		"pushover API error: invalid user",
		"pushover API error: user identifier is not a valid user, group, or subscribed user key",
		"pushover API error: user not found",
		"pushover API error: no such user",
	}
	for _, msg := range gone {
		if !isSubscribedUserGoneError(&testError{msg: msg}) {
			t.Errorf("expected gone for %q", msg)
		}
	}

	keep := []string{
		"pushover API error: application token is invalid",
		"pushover API error: subscription not found",
		"pushover API returned HTTP 404",
		"pushover API error: not found",
		"pushover API error: device name is invalid",
		"pushover API returned HTTP 500 after 4 attempts",
	}
	for _, msg := range keep {
		if isSubscribedUserGoneError(&testError{msg: msg}) {
			t.Errorf("expected keep for %q", msg)
		}
	}
	if isSubscribedUserGoneError(nil) {
		t.Error("nil error is not a gone subscriber")
	}
}

// withUnknownComputed marks id and migration outputs unknown, as they are during plan.
func withUnknownComputed(val tftypes.Value) (tftypes.Value, error) {
	return tftypes.Transform(val, func(path *tftypes.AttributePath, v tftypes.Value) (tftypes.Value, error) {
		name, ok := path.LastStep().(tftypes.AttributeName)
		if !ok {
			return v, nil
		}
		switch string(name) {
		case "id", "subscribed_user_key", "request_id":
			return tftypes.NewValue(tftypes.String, tftypes.UnknownValue), nil
		default:
			return v, nil
		}
	})
}

func subscriptionSubscriberSchema(t *testing.T) resource.SchemaResponse {
	t.Helper()
	res := &SubscriptionSubscriberResource{}
	schemaResp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema errors: %v", schemaResp.Diagnostics)
	}
	return *schemaResp
}

func subscriptionSubscriberState(t *testing.T, schemaResp resource.SchemaResponse, device, sound, token *string) tfsdk.State {
	t.Helper()
	ctx := context.Background()
	tfType := schemaResp.Schema.Type().TerraformType(ctx)

	deviceVal := tftypes.NewValue(tftypes.String, nil)
	if device != nil {
		deviceVal = tftypes.NewValue(tftypes.String, *device)
	}
	soundVal := tftypes.NewValue(tftypes.String, nil)
	if sound != nil {
		soundVal = tftypes.NewValue(tftypes.String, *sound)
	}
	tokenVal := tftypes.NewValue(tftypes.String, nil)
	if token != nil {
		tokenVal = tftypes.NewValue(tftypes.String, *token)
	}

	return tfsdk.State{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
			"id":                  tftypes.NewValue(tftypes.String, "alerts-a1b2c3/uKey"),
			"subscription":        tftypes.NewValue(tftypes.String, "alerts-a1b2c3"),
			"user_key":            tftypes.NewValue(tftypes.String, "uKey"),
			"device_name":         deviceVal,
			"sound":               soundVal,
			"api_token":           tokenVal,
			"subscribed_user_key": tftypes.NewValue(tftypes.String, "sPfjsD2fGzEd9TR52DU31Hv4A61Vvk"),
			"request_id":          tftypes.NewValue(tftypes.String, "migrate-req"),
		}),
	}
}

func TestSubscriptionSubscriberResource_Create_Success(t *testing.T) {
	t.Parallel()

	var gotPath, gotUser, gotDevice, gotSound, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			return
		}
		gotUser = r.FormValue("user")
		gotDevice = r.FormValue("device_name")
		gotSound = r.FormValue("sound")
		gotToken = r.FormValue("token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"request":"migrate-req","subscribed_user_key":"sPfjsD2fGzEd9TR52DU31Hv4A61Vvk"}`))
	}))
	defer srv.Close()

	schemaResp := subscriptionSubscriberSchema(t)
	ctx := context.Background()
	device := "iphone"
	sound := "pushover"
	planState := subscriptionSubscriberState(t, schemaResp, &device, &sound, nil)
	planRaw, err := withUnknownComputed(planState.Raw)
	if err != nil {
		t.Fatalf("transform plan: %v", err)
	}

	res := &SubscriptionSubscriberResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics: %v", resp.Diagnostics)
	}

	if gotPath != "/subscriptions/migrate.json" {
		t.Errorf("unexpected path %s", gotPath)
	}
	if gotUser != "uKey" || gotDevice != "iphone" || gotSound != "pushover" || gotToken != "app-token" {
		t.Errorf("unexpected form user=%q device=%q sound=%q token=%q", gotUser, gotDevice, gotSound, gotToken)
	}

	var got SubscriptionSubscriberResourceModel
	diags := resp.State.Get(ctx, &got)
	if diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.SubscribedUserKey.ValueString() != "sPfjsD2fGzEd9TR52DU31Hv4A61Vvk" {
		t.Errorf("subscribed key %q", got.SubscribedUserKey.ValueString())
	}
	if got.ID.ValueString() != "alerts-a1b2c3/uKey/iphone" {
		t.Errorf("id %q", got.ID.ValueString())
	}
	if got.RequestID.ValueString() != "migrate-req" {
		t.Errorf("request id %q", got.RequestID.ValueString())
	}
	if got.Sound.ValueString() != "pushover" {
		t.Errorf("sound %q", got.Sound.ValueString())
	}
}

func TestSubscriptionSubscriberResource_Create_APIErrorLeavesStateUnset(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"errors":["subscription not found"],"request":"req-err"}`))
	}))
	defer srv.Close()

	schemaResp := subscriptionSubscriberSchema(t)
	ctx := context.Background()
	state := subscriptionSubscriberState(t, schemaResp, nil, nil, nil)
	planRaw, err := withUnknownComputed(state.Raw)
	if err != nil {
		t.Fatalf("transform plan: %v", err)
	}

	res := &SubscriptionSubscriberResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: planRaw}}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected create error")
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected no state after failed create")
	}
}

func TestSubscriptionSubscriberResource_Read_DropsInvalidSubscribedKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/validate.json" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"errors":["user identifier is not a valid user, group, or subscribed user key"],"request":"v"}`))
	}))
	defer srv.Close()

	schemaResp := subscriptionSubscriberSchema(t)
	state := subscriptionSubscriberState(t, schemaResp, nil, nil, nil)
	res := &SubscriptionSubscriberResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state removed when subscribed user key is invalid")
	}
}

func TestSubscriptionSubscriberResource_Read_KeepsStateOnUnrelatedError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"errors":["application token is invalid"],"request":"v"}`))
	}))
	defer srv.Close()

	schemaResp := subscriptionSubscriberSchema(t)
	state := subscriptionSubscriberState(t, schemaResp, nil, nil, nil)
	res := &SubscriptionSubscriberResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected refresh error")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("unrelated API errors must keep state")
	}

	var got SubscriptionSubscriberResourceModel
	diags := resp.State.Get(context.Background(), &got)
	if diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.SubscribedUserKey.ValueString() != "sPfjsD2fGzEd9TR52DU31Hv4A61Vvk" {
		t.Errorf("subscribed key changed to %q", got.SubscribedUserKey.ValueString())
	}
	if got.RequestID.ValueString() != "migrate-req" {
		t.Errorf("request id changed to %q", got.RequestID.ValueString())
	}
}

func TestSubscriptionSubscriberResource_Read_ValidKeyLeavesConfig(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			return
		}
		if r.FormValue("user") != "sPfjsD2fGzEd9TR52DU31Hv4A61Vvk" {
			t.Errorf("validated unexpected user %q", r.FormValue("user"))
		}
		if r.FormValue("token") != "override-token" {
			t.Errorf("expected token override, got %q", r.FormValue("token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"request":"validate-req","group":0,"devices":["iphone"],"licenses":["Android"]}`))
	}))
	defer srv.Close()

	schemaResp := subscriptionSubscriberSchema(t)
	device := "iphone"
	sound := "bike"
	token := "override-token"
	state := subscriptionSubscriberState(t, schemaResp, &device, &sound, &token)
	res := &SubscriptionSubscriberResource{
		client: pushover.NewClientWithBase("provider-token", srv.URL, srv.Client()),
	}
	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to remain")
	}

	var got SubscriptionSubscriberResourceModel
	diags := resp.State.Get(context.Background(), &got)
	if diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.Sound.ValueString() != "bike" || got.DeviceName.ValueString() != "iphone" {
		t.Errorf("refresh rewrote config sound=%q device=%q", got.Sound.ValueString(), got.DeviceName.ValueString())
	}
	if got.RequestID.ValueString() != "migrate-req" {
		t.Errorf("refresh rewrote request id to %q", got.RequestID.ValueString())
	}
}

func TestSubscriptionSubscriberResource_Delete_DoesNotCallAPI(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	schemaResp := subscriptionSubscriberSchema(t)
	state := subscriptionSubscriberState(t, schemaResp, nil, nil, nil)
	res := &SubscriptionSubscriberResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
	resp := &resource.DeleteResponse{State: state}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}
