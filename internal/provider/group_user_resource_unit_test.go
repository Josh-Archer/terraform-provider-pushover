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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestParseGroupUserImportID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		wantGroup  string
		wantUser   string
		wantDevice string
		wantErr    bool
	}{
		{
			name:      "group and user only",
			id:        "gKey/uKey",
			wantGroup: "gKey",
			wantUser:  "uKey",
		},
		{
			name:       "with device",
			id:         "gKey/uKey/iphone",
			wantGroup:  "gKey",
			wantUser:   "uKey",
			wantDevice: "iphone",
		},
		{
			name:    "empty",
			id:      "",
			wantErr: true,
		},
		{
			name:    "too few parts",
			id:      "onlygroup",
			wantErr: true,
		},
		{
			name:    "too many parts",
			id:      "a/b/c/d",
			wantErr: true,
		},
		{
			name:    "empty user",
			id:      "gKey/",
			wantErr: true,
		},
		{
			name:    "empty device segment",
			id:      "gKey/uKey/",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			g, u, d, err := parseGroupUserImportID(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for id %q", tt.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g != tt.wantGroup || u != tt.wantUser || d != tt.wantDevice {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)",
					g, u, d, tt.wantGroup, tt.wantUser, tt.wantDevice)
			}
		})
	}
}

func TestGroupUserResourceID(t *testing.T) {
	t.Parallel()
	if got := groupUserResourceID("g", "u", ""); got != "g/u" {
		t.Errorf("without device: got %q", got)
	}
	if got := groupUserResourceID("g", "u", "iphone"); got != "g/u/iphone" {
		t.Errorf("with device: got %q", got)
	}
}

func TestFindGroupMember_NotFound(t *testing.T) {
	t.Parallel()

	members := []pushover.GroupMember{
		{User: "u1", Device: "", Disabled: false, Memo: "a"},
		{User: "u2", Device: "iphone", Disabled: true, Memo: "b"},
	}

	// Missing user entirely.
	if _, found := findGroupMember(members, "missing", ""); found {
		t.Fatal("expected missing user not found")
	}

	// Same user, wrong device — device-scoped membership must not match bare membership.
	if _, found := findGroupMember(members, "u2", ""); found {
		t.Fatal("expected device-scoped member not to match empty device lookup")
	}

	// Bare membership must not match a device-scoped query.
	if _, found := findGroupMember(members, "u1", "iphone"); found {
		t.Fatal("expected empty-device member not to match device lookup")
	}
}

func TestFindGroupMember_FoundAndDisabledDrift(t *testing.T) {
	t.Parallel()

	members := []pushover.GroupMember{
		{User: "u1", Device: "", Disabled: true, Memo: "on leave"},
		{User: "u2", Device: "pixel", Disabled: false, Memo: ""},
	}

	m, found := findGroupMember(members, "u1", "")
	if !found {
		t.Fatal("expected to find u1")
	}
	if !m.Disabled {
		t.Error("expected disabled=true from remote membership")
	}
	if m.Memo != "on leave" {
		t.Errorf("unexpected memo: %q", m.Memo)
	}

	m, found = findGroupMember(members, "u2", "pixel")
	if !found {
		t.Fatal("expected to find u2/pixel")
	}
	if m.Disabled {
		t.Error("expected disabled=false")
	}
}

func TestApplyGroupMemberToModel_SyncsDisabledAndMemo(t *testing.T) {
	t.Parallel()

	data := &GroupUserResourceModel{
		GroupKey: types.StringValue("g"),
		UserKey:  types.StringValue("u"),
		Disabled: types.BoolValue(false),
		Memo:     types.StringValue("old memo"),
	}

	applyGroupMemberToModel(data, pushover.GroupMember{
		User:     "u",
		Disabled: true,
		Memo:     "new memo",
	})

	if !data.Disabled.ValueBool() {
		t.Error("expected disabled synced to true")
	}
	if data.Memo.ValueString() != "new memo" {
		t.Errorf("expected memo synced, got %q", data.Memo.ValueString())
	}

	// Remote memo cleared.
	applyGroupMemberToModel(data, pushover.GroupMember{
		User:     "u",
		Disabled: false,
		Memo:     "",
	})
	if !data.Memo.IsNull() {
		t.Errorf("expected memo null after remote clear, got %#v", data.Memo)
	}
	if data.Disabled.ValueBool() {
		t.Error("expected disabled synced to false")
	}
}

func TestIsGroupNotFoundError(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"pushover API error: group not found": true,
		"pushover API error: not found":       true,
		"pushover API error: invalid group":   true,
		"pushover API error: rate limited":    false,
		"": false,
	}
	for msg, want := range cases {
		var err error
		if msg != "" {
			err = &testError{msg: msg}
		}
		if got := isGroupNotFoundError(err); got != want {
			t.Errorf("isGroupNotFoundError(%q)=%v, want %v", msg, got, want)
		}
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// TestGroupUserResource_Read_RemovesMissingMembership verifies Read drops state
// when the user is no longer a member of the group (external removal).
func TestGroupUserResource_Read_RemovesMissingMembership(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]interface{}{
			"status":  1,
			"request": "req-1",
			"name":    "ops",
			// Target user is absent — membership was removed outside Terraform.
			"users": []map[string]interface{}{
				{"user": "other_user", "device": "", "memo": "", "disabled": false},
			},
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	r := &GroupUserResource{
		client: pushover.NewClientWithBase("tok", srv.URL, srv.Client()),
	}

	schemaResp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema errors: %v", schemaResp.Diagnostics)
	}

	state := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":        tftypes.NewValue(tftypes.String, "gKey/uKey"),
			"group_key": tftypes.NewValue(tftypes.String, "gKey"),
			"user_key":  tftypes.NewValue(tftypes.String, "uKey"),
			"device":    tftypes.NewValue(tftypes.String, nil),
			"memo":      tftypes.NewValue(tftypes.String, "note"),
			"disabled":  tftypes.NewValue(tftypes.Bool, false),
		}),
	}

	resp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be removed when membership is missing")
	}
}

// TestGroupUserResource_Read_SyncsDisabledDrift verifies enable/disable changes
// made outside Terraform are reflected on refresh.
func TestGroupUserResource_Read_SyncsDisabledDrift(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]interface{}{
			"status":  1,
			"request": "req-2",
			"name":    "ops",
			"users": []map[string]interface{}{
				{"user": "uKey", "device": "", "memo": "remote memo", "disabled": true},
			},
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	res := &GroupUserResource{
		client: pushover.NewClientWithBase("tok", srv.URL, srv.Client()),
	}

	schemaResp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	state := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":        tftypes.NewValue(tftypes.String, "gKey/uKey"),
			"group_key": tftypes.NewValue(tftypes.String, "gKey"),
			"user_key":  tftypes.NewValue(tftypes.String, "uKey"),
			"device":    tftypes.NewValue(tftypes.String, nil),
			"memo":      tftypes.NewValue(tftypes.String, "local memo"),
			"disabled":  tftypes.NewValue(tftypes.Bool, false), // local says enabled
		}),
	}

	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to remain when membership exists")
	}

	var got GroupUserResourceModel
	diags := resp.State.Get(context.Background(), &got)
	if diags.HasError() {
		t.Fatalf("state get errors: %v", diags)
	}
	if !got.Disabled.ValueBool() {
		t.Error("expected disabled=true after refresh of remote disable")
	}
	if got.Memo.ValueString() != "remote memo" {
		t.Errorf("expected remote memo, got %q", got.Memo.ValueString())
	}
}

// TestGroupUserResource_Read_GroupNotFoundRemovesState treats a missing group
// as absent membership.
func TestGroupUserResource_Read_GroupNotFoundRemovesState(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":0,"errors":["group not found"],"request":"r"}`))
	}))
	defer srv.Close()

	res := &GroupUserResource{
		client: pushover.NewClientWithBase("tok", srv.URL, srv.Client()),
	}

	schemaResp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)

	state := tfsdk.State{
		Schema: schemaResp.Schema,
		Raw: tftypes.NewValue(schemaResp.Schema.Type().TerraformType(context.Background()), map[string]tftypes.Value{
			"id":        tftypes.NewValue(tftypes.String, "gKey/uKey"),
			"group_key": tftypes.NewValue(tftypes.String, "gKey"),
			"user_key":  tftypes.NewValue(tftypes.String, "uKey"),
			"device":    tftypes.NewValue(tftypes.String, nil),
			"memo":      tftypes.NewValue(tftypes.String, nil),
			"disabled":  tftypes.NewValue(tftypes.Bool, false),
		}),
	}

	resp := &resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state removed when group not found")
	}
}
