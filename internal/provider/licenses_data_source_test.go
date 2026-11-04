// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestLicensesDataSource_Schema(t *testing.T) {
	t.Parallel()

	ds := NewLicensesDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema errors: %v", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "api_token", "credits", "request_id"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q in schema", attr)
		}
	}
}

func TestLicensesDataSource_Read_SuccessWithTokenOverride(t *testing.T) {
	t.Parallel()

	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("token")
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/licenses.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"credits":5,"request":"142f8378-fd19-4cb7-9ea2-b18b15a07431"}`))
	}))
	defer srv.Close()

	ds := &LicensesDataSource{
		client: pushover.NewClientWithBase("default-token", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema errors: %v", schemaResp.Diagnostics)
	}

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":         tftypes.NewValue(tftypes.String, nil),
			"api_token":  tftypes.NewValue(tftypes.String, "override-token"),
			"credits":    tftypes.NewValue(tftypes.Number, nil),
			"request_id": tftypes.NewValue(tftypes.String, nil),
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

	var state LicensesDataSourceModel
	diags := readResp.State.Get(context.Background(), &state)
	if diags.HasError() {
		t.Fatalf("state get diags: %v", diags)
	}

	if state.ID.ValueString() != "licenses" {
		t.Errorf("expected id 'licenses', got %q", state.ID.ValueString())
	}
	if state.ID.ValueString() == "override-token" {
		t.Errorf("expected id not to expose sensitive api_token override")
	}
	if state.Credits.ValueInt64() != 5 {
		t.Errorf("expected credits 5, got %d", state.Credits.ValueInt64())
	}
	if state.RequestID.ValueString() != "142f8378-fd19-4cb7-9ea2-b18b15a07431" {
		t.Errorf("expected request_id '142f8378-fd19-4cb7-9ea2-b18b15a07431', got %q", state.RequestID.ValueString())
	}
}

func TestLicensesDataSource_Read_DefaultToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "default-token" {
			t.Errorf("expected default token, got %q", r.URL.Query().Get("token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"credits":42,"request":"req-lic-1"}`))
	}))
	defer srv.Close()

	ds := &LicensesDataSource{
		client: pushover.NewClientWithBase("default-token", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":         tftypes.NewValue(tftypes.String, nil),
			"api_token":  tftypes.NewValue(tftypes.String, nil),
			"credits":    tftypes.NewValue(tftypes.Number, nil),
			"request_id": tftypes.NewValue(tftypes.String, nil),
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

	var state LicensesDataSourceModel
	diags := readResp.State.Get(context.Background(), &state)
	if diags.HasError() {
		t.Fatalf("state get diags: %v", diags)
	}

	if state.ID.ValueString() != "licenses" {
		t.Errorf("expected id 'licenses', got %q", state.ID.ValueString())
	}
	if state.Credits.ValueInt64() != 42 {
		t.Errorf("expected credits 42, got %d", state.Credits.ValueInt64())
	}
}

func TestLicensesDataSource_Read_Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"status":0,"errors":["application token is invalid"]}`))
	}))
	defer srv.Close()

	ds := &LicensesDataSource{
		client: pushover.NewClientWithBase("invalid-tok", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":         tftypes.NewValue(tftypes.String, nil),
			"api_token":  tftypes.NewValue(tftypes.String, nil),
			"credits":    tftypes.NewValue(tftypes.Number, nil),
			"request_id": tftypes.NewValue(tftypes.String, nil),
		}),
	}

	readReq := datasource.ReadRequest{Config: cfg}
	readResp := &datasource.ReadResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}

	ds.Read(context.Background(), readReq, readResp)

	if !readResp.Diagnostics.HasError() {
		t.Fatalf("expected error on invalid token, got none")
	}
}
