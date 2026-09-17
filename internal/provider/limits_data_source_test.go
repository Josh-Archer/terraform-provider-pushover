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

func TestLimitsDataSource_Schema(t *testing.T) {
	t.Parallel()

	ds := NewLimitsDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema errors: %v", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "api_token", "limit", "remaining", "reset"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q in schema", attr)
		}
	}
}

func TestLimitsDataSource_Read_Success(t *testing.T) {
	t.Parallel()

	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("token")
		if r.URL.Path != "/apps/limits.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"status":    1,
			"request":   "req-lim-1",
			"limit":     10000,
			"remaining": 7496,
			"reset":     1393653600,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ds := &LimitsDataSource{
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
			"id":        tftypes.NewValue(tftypes.String, nil),
			"api_token": tftypes.NewValue(tftypes.String, "override-token"),
			"limit":     tftypes.NewValue(tftypes.Number, nil),
			"remaining": tftypes.NewValue(tftypes.Number, nil),
			"reset":     tftypes.NewValue(tftypes.Number, nil),
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

	var state LimitsDataSourceModel
	diags := readResp.State.Get(context.Background(), &state)
	if diags.HasError() {
		t.Fatalf("state get diags: %v", diags)
	}

	if state.ID.ValueString() != "override-token" {
		t.Errorf("expected id 'override-token', got %q", state.ID.ValueString())
	}
	if state.Limit.ValueInt64() != 10000 {
		t.Errorf("expected limit 10000, got %d", state.Limit.ValueInt64())
	}
	if state.Remaining.ValueInt64() != 7496 {
		t.Errorf("expected remaining 7496, got %d", state.Remaining.ValueInt64())
	}
	if state.Reset.ValueInt64() != 1393653600 {
		t.Errorf("expected reset 1393653600, got %d", state.Reset.ValueInt64())
	}
}

func TestLimitsDataSource_Read_DefaultToken(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "default-token" {
			t.Errorf("expected default token, got %q", r.URL.Query().Get("token"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":1,"request":"req-1","limit":10000,"remaining":9999,"reset":1393653600}`))
	}))
	defer srv.Close()

	ds := &LimitsDataSource{
		client: pushover.NewClientWithBase("default-token", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":        tftypes.NewValue(tftypes.String, nil),
			"api_token": tftypes.NewValue(tftypes.String, nil),
			"limit":     tftypes.NewValue(tftypes.Number, nil),
			"remaining": tftypes.NewValue(tftypes.Number, nil),
			"reset":     tftypes.NewValue(tftypes.Number, nil),
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

	var state LimitsDataSourceModel
	diags := readResp.State.Get(context.Background(), &state)
	if diags.HasError() {
		t.Fatalf("state get diags: %v", diags)
	}

	if state.ID.ValueString() != "limits" {
		t.Errorf("expected id 'limits', got %q", state.ID.ValueString())
	}
	if state.Remaining.ValueInt64() != 9999 {
		t.Errorf("expected remaining 9999, got %d", state.Remaining.ValueInt64())
	}
}

func TestLimitsDataSource_Read_Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":0,"errors":["application token is invalid"]}`))
	}))
	defer srv.Close()

	ds := &LimitsDataSource{
		client: pushover.NewClientWithBase("invalid-tok", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":        tftypes.NewValue(tftypes.String, nil),
			"api_token": tftypes.NewValue(tftypes.String, nil),
			"limit":     tftypes.NewValue(tftypes.Number, nil),
			"remaining": tftypes.NewValue(tftypes.Number, nil),
			"reset":     tftypes.NewValue(tftypes.Number, nil),
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
