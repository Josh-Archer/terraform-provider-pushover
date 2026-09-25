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

func TestSoundsDataSource_Schema(t *testing.T) {
	t.Parallel()

	ds := NewSoundsDataSource()
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema errors: %v", resp.Diagnostics)
	}

	for _, attr := range []string{"id", "sounds", "keys"} {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected attribute %q in schema", attr)
		}
	}
}

func TestSoundsDataSource_Configure(t *testing.T) {
	t.Parallel()

	ds := NewSoundsDataSource().(*SoundsDataSource)
	resp := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "invalid"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatalf("expected configure error for invalid provider data, got none")
	}

	respNil := &datasource.ConfigureResponse{}
	ds.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, respNil)
	if respNil.Diagnostics.HasError() {
		t.Fatalf("unexpected configure error for nil provider data: %v", respNil.Diagnostics)
	}
}

func TestSoundsDataSource_Read_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sounds.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		resp := map[string]interface{}{
			"status":  1,
			"request": "req-sounds-1",
			"sounds": map[string]string{
				"pushover":     "Pushover (default)",
				"bike":         "Bike",
				"bugle":        "Bugle",
				"cashregister": "Cash Register",
				"classical":    "Classical",
				"cosmic":       "Cosmic",
				"falling":      "Falling",
				"gamelan":      "Gamelan",
				"incoming":     "Incoming",
				"intermission": "Intermission",
				"magic":        "Magic",
				"mechanical":   "Mechanical",
				"pianobar":     "Piano Bar",
				"siren":        "Siren",
				"spacealarm":   "Space Alarm",
				"tugboat":      "Tug Boat",
				"alien":        "Alien Alarm",
				"climb":        "Climb",
				"persistent":   "Persistent",
				"echo":         "Pushover Echo",
				"updown":       "Up Down",
				"none":         "None",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ds := &SoundsDataSource{
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
			"id":     tftypes.NewValue(tftypes.String, nil),
			"sounds": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil),
			"keys":   tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
		}),
	}

	expectedSortedKeys := []string{
		"alien", "bike", "bugle", "cashregister", "classical", "climb", "cosmic",
		"echo", "falling", "gamelan", "incoming", "intermission", "magic",
		"mechanical", "none", "persistent", "pianobar", "pushover", "siren",
		"spacealarm", "tugboat", "updown",
	}

	// Run multiple times to verify deterministic sorting across plans.
	for iter := 0; iter < 10; iter++ {
		readReq := datasource.ReadRequest{Config: cfg}
		readResp := &datasource.ReadResponse{
			State: tfsdk.State{Schema: schemaResp.Schema},
		}

		ds.Read(context.Background(), readReq, readResp)

		if readResp.Diagnostics.HasError() {
			t.Fatalf("iteration %d: read diagnostics: %v", iter, readResp.Diagnostics)
		}

		var state SoundsDataSourceModel
		diags := readResp.State.Get(context.Background(), &state)
		if diags.HasError() {
			t.Fatalf("iteration %d: state get diags: %v", iter, diags)
		}

		if state.ID.ValueString() != "sounds" {
			t.Errorf("iteration %d: expected id 'sounds', got %q", iter, state.ID.ValueString())
		}

		var keys []string
		diags = state.Keys.ElementsAs(context.Background(), &keys, false)
		if diags.HasError() {
			t.Fatalf("iteration %d: keys elementsAs diags: %v", iter, diags)
		}

		if len(keys) != len(expectedSortedKeys) {
			t.Fatalf("iteration %d: expected %d keys, got %d", iter, len(expectedSortedKeys), len(keys))
		}

		for i, expected := range expectedSortedKeys {
			if keys[i] != expected {
				t.Errorf("iteration %d: keys[%d] = %q, expected %q", iter, i, keys[i], expected)
			}
		}
	}
}

func TestSoundsDataSource_Read_Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":0,"errors":["application token is invalid"]}`))
	}))
	defer srv.Close()

	ds := &SoundsDataSource{
		client: pushover.NewClientWithBase("invalid-tok", srv.URL, srv.Client()),
	}

	schemaResp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), datasource.SchemaRequest{}, schemaResp)

	cfg := tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":     tftypes.NewValue(tftypes.String, nil),
			"sounds": tftypes.NewValue(tftypes.Map{ElementType: tftypes.String}, nil),
			"keys":   tftypes.NewValue(tftypes.List{ElementType: tftypes.String}, nil),
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
