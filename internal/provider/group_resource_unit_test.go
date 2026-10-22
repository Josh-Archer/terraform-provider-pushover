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
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestGroupResource_Schema(t *testing.T) {
	t.Parallel()

	resp := &resource.SchemaResponse{}
	NewGroupResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected schema errors: %v", resp.Diagnostics)
	}
	for _, name := range []string{"id", "group_key", "name"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("expected attribute %q", name)
		}
	}
}

func TestGroupResource_CreateAndUpdateRenameGroup(t *testing.T) {
	t.Parallel()

	var postedNames []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/groups/gKey/rename.json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		postedNames = append(postedNames, r.FormValue("name"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":1,"request":"req"}`))
	}))
	defer srv.Close()

	res := &GroupResource{client: pushover.NewClientWithBase("tok", srv.URL, srv.Client())}
	schemaResp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	plan := groupResourcePlan(t, schemaResp.Schema, "gKey", "Operations")
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	res.Create(context.Background(), resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create diagnostics: %v", createResp.Diagnostics)
	}
	var created GroupResourceModel
	if diags := createResp.State.Get(context.Background(), &created); diags.HasError() {
		t.Fatalf("read created state: %v", diags)
	}
	if created.ID.ValueString() != "gKey" || created.Name.ValueString() != "Operations" {
		t.Fatalf("unexpected created state: id=%q name=%q", created.ID.ValueString(), created.Name.ValueString())
	}

	updatePlan := groupResourcePlan(t, schemaResp.Schema, "gKey", "On Call")
	updateResp := &resource.UpdateResponse{State: createResp.State}
	res.Update(context.Background(), resource.UpdateRequest{Plan: updatePlan}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update diagnostics: %v", updateResp.Diagnostics)
	}
	if len(postedNames) != 2 || postedNames[0] != "Operations" || postedNames[1] != "On Call" {
		t.Fatalf("unexpected rename calls: %v", postedNames)
	}
}

func TestGroupResource_ReadSyncsRemoteName(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/groups/gKey.json" || r.URL.Query().Get("token") != "tok" {
			t.Errorf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": 1, "request": "req", "name": "Remote Name"})
	}))
	defer srv.Close()

	res := &GroupResource{client: pushover.NewClientWithBase("tok", srv.URL, srv.Client())}
	schemaResp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	state := groupResourceState(t, schemaResp.Schema, "gKey", "Stale Name")
	readResp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read diagnostics: %v", readResp.Diagnostics)
	}
	var got GroupResourceModel
	if diags := readResp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("read state: %v", diags)
	}
	if got.Name.ValueString() != "Remote Name" {
		t.Errorf("expected remote name, got %q", got.Name.ValueString())
	}
}

func groupResourcePlan(t *testing.T, resourceSchema schema.Schema, groupKey, name string) tfsdk.Plan {
	t.Helper()
	return tfsdk.Plan{Schema: resourceSchema, Raw: groupResourceRaw(groupKey, name)}
}

func groupResourceState(t *testing.T, resourceSchema schema.Schema, groupKey, name string) tfsdk.State {
	t.Helper()
	return tfsdk.State{Schema: resourceSchema, Raw: groupResourceRaw(groupKey, name)}
}

func groupResourceRaw(groupKey, name string) tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"id": tftypes.String, "group_key": tftypes.String, "name": tftypes.String,
	}}, map[string]tftypes.Value{
		"id":        tftypes.NewValue(tftypes.String, groupKey),
		"group_key": tftypes.NewValue(tftypes.String, groupKey),
		"name":      tftypes.NewValue(tftypes.String, name),
	})
}
