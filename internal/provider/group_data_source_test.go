// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestGroupDataSource_Schema(t *testing.T) {
	t.Parallel()

	ds := NewGroupDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema errors: %v", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "group_key", "api_token", "name", "users"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q in schema", attr)
		}
	}
}

func TestGroupDataSource_Read_Success(t *testing.T) {
	t.Parallel()

	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("token")
		if r.URL.Path != "/groups/group123.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"status":  1,
			"request": "req-1",
			"name":    "On-Call Engineers",
			"users": []map[string]interface{}{
				{
					"user":     "user1",
					"device":   "phone",
					"memo":     "Primary",
					"disabled": false,
				},
				{
					"user":     "user2",
					"device":   "",
					"memo":     "",
					"disabled": true,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ds := &GroupDataSource{
		client: pushover.NewClientWithBase("default-tok", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema errors: %v", schemaResp.Diagnostics)
	}

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":        tftypes.NewValue(tftypes.String, nil),
			"group_key": tftypes.NewValue(tftypes.String, "group123"),
			"api_token": tftypes.NewValue(tftypes.String, "override-token"),
			"name":      tftypes.NewValue(tftypes.String, nil),
			"users":     tftypes.NewValue(schemaResp.Schema.Attributes["users"].GetType().TerraformType(context.Background()), nil),
		}),
	}

	readReq := datasource.ReadRequest{Config: cfg}
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}

	ds.Read(context.Background(), readReq, readResp)

	if readResp.Diagnostics.HasError() {
		t.Fatalf("read diagnostics: %v", readResp.Diagnostics)
	}

	if gotToken != "override-token" {
		t.Errorf("expected token 'override-token', got %q", gotToken)
	}

	var state GroupDataSourceModel
	diags := readResp.State.Get(context.Background(), &state)
	if diags.HasError() {
		t.Fatalf("state get diags: %v", diags)
	}

	if state.ID.ValueString() != "group123" {
		t.Errorf("expected id 'group123', got %q", state.ID.ValueString())
	}
	if state.Name.ValueString() != "On-Call Engineers" {
		t.Errorf("expected name 'On-Call Engineers', got %q", state.Name.ValueString())
	}
	if len(state.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(state.Users))
	}

	// User 1
	if state.Users[0].UserKey.ValueString() != "user1" {
		t.Errorf("expected user1, got %s", state.Users[0].UserKey.ValueString())
	}
	if state.Users[0].Device.ValueString() != "phone" {
		t.Errorf("expected phone, got %s", state.Users[0].Device.ValueString())
	}
	if state.Users[0].Memo.ValueString() != "Primary" {
		t.Errorf("expected Primary, got %s", state.Users[0].Memo.ValueString())
	}
	if state.Users[0].Disabled.ValueBool() {
		t.Errorf("expected disabled false for user1")
	}

	// User 2
	if state.Users[1].UserKey.ValueString() != "user2" {
		t.Errorf("expected user2, got %s", state.Users[1].UserKey.ValueString())
	}
	if !state.Users[1].Device.IsNull() {
		t.Errorf("expected null device for user2, got %v", state.Users[1].Device)
	}
	if !state.Users[1].Memo.IsNull() {
		t.Errorf("expected null memo for user2, got %v", state.Users[1].Memo)
	}
	if !state.Users[1].Disabled.ValueBool() {
		t.Errorf("expected disabled true for user2")
	}
}

func TestGroupDataSource_Read_NotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":0,"errors":["group not found"]}`))
	}))
	defer srv.Close()

	ds := &GroupDataSource{
		client: pushover.NewClientWithBase("default-tok", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":        tftypes.NewValue(tftypes.String, nil),
			"group_key": tftypes.NewValue(tftypes.String, "nonexistent"),
			"api_token": tftypes.NewValue(tftypes.String, nil),
			"name":      tftypes.NewValue(tftypes.String, nil),
			"users":     tftypes.NewValue(schemaResp.Schema.Attributes["users"].GetType().TerraformType(context.Background()), nil),
		}),
	}

	readReq := datasource.ReadRequest{Config: cfg}
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}

	ds.Read(context.Background(), readReq, readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatalf("expected error for nonexistent group, got none")
	}
}
