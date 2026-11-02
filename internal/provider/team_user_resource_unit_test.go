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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestParseTeamUserImportID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		id        string
		wantToken string
		wantUser  string
		wantEmail string
		wantErr   bool
	}{
		{name: "user id", id: "team-token/user-id-1", wantToken: "team-token", wantUser: "user-id-1"},
		{name: "email", id: "team-token/ada@example.com", wantToken: "team-token", wantEmail: "ada@example.com"},
		{name: "trimmed", id: "  team-token/user-id-1  ", wantToken: "team-token", wantUser: "user-id-1"},
		{name: "empty", id: "", wantErr: true},
		{name: "no slash", id: "team-token", wantErr: true},
		{name: "empty user", id: "team-token/", wantErr: true},
		{name: "empty token", id: "/user-id-1", wantErr: true},
		{name: "too many parts", id: "team-token/user-id-1/extra", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			token, userID, email, err := parseTeamUserImportID(tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for id %q", tt.id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token != tt.wantToken || userID != tt.wantUser || email != tt.wantEmail {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", token, userID, email, tt.wantToken, tt.wantUser, tt.wantEmail)
			}
		})
	}
}

func TestTeamUserResourceID(t *testing.T) {
	t.Parallel()
	if got := teamUserResourceID("team-token", "user-id-1"); got != "team-token/user-id-1" {
		t.Fatalf("got %q", got)
	}
}

func TestFindTeamUser(t *testing.T) {
	t.Parallel()

	users := []pushover.TeamUser{
		{ID: "user-1", Email: "Ada@Example.com", Name: "Ada", Administrator: true},
		{ID: "user-2", Email: "bob@example.com", Name: "Bob", Administrator: false},
	}

	got, found := findTeamUser(users, "user-2", "ada@example.com")
	if !found || got.ID != "user-2" {
		t.Fatalf("user id lookup failed: found=%v id=%s", found, got.ID)
	}
	if _, found := findTeamUser(users, "missing", "ada@example.com"); found {
		t.Fatal("known user id must not fall back to a different email")
	}
	got, found = findTeamUser(users, "", "ada@example.com")
	if !found || got.ID != "user-1" {
		t.Fatal("expected case-insensitive email match")
	}
	if _, found := findTeamUser(users, "", "missing@example.com"); found {
		t.Fatal("expected missing email")
	}
	if _, found := findTeamUser(users, "", ""); found {
		t.Fatal("expected empty lookup to miss")
	}
}

func TestApplyTeamUserToModel(t *testing.T) {
	t.Parallel()

	data := &TeamUserResourceModel{
		Token:    types.StringValue("team-token"),
		Email:    types.StringValue("Ada@Example.com"),
		Name:     types.StringValue("Old"),
		Password: types.StringValue("kept-password"),
		Instant:  types.BoolValue(true),
		Admin:    types.BoolValue(false),
		Group:    types.StringValue("Ops"),
	}
	applyTeamUserToModel(data, pushover.TeamUser{
		ID:            "user-1",
		Email:         "ada@example.com",
		Name:          "New Name",
		Administrator: true,
	})
	if data.UserID.ValueString() != "user-1" {
		t.Errorf("user id: %s", data.UserID.ValueString())
	}
	if data.Email.ValueString() != "Ada@Example.com" {
		t.Errorf("email case should be preserved, got %s", data.Email.ValueString())
	}
	if !data.Admin.ValueBool() {
		t.Error("expected admin synced to true")
	}
	if data.Name.ValueString() != "New Name" {
		t.Errorf("name: %s", data.Name.ValueString())
	}
	if data.Password.ValueString() != "kept-password" || !data.Instant.ValueBool() || data.Group.ValueString() != "Ops" {
		t.Error("create-time attributes should be left unchanged")
	}

	unset := &TeamUserResourceModel{Name: types.StringNull(), Email: types.StringNull()}
	applyTeamUserToModel(unset, pushover.TeamUser{ID: "user-9", Email: "new@example.com", Name: "Remote", Administrator: false})
	if !unset.Name.IsNull() {
		t.Fatal("unset name must stay null so refresh does not plan a replacement")
	}
	if unset.Email.ValueString() != "new@example.com" {
		t.Fatalf("email: %s", unset.Email.ValueString())
	}
}

func TestIsAlreadyOnTeamError(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"pushover API error: user is already a member of this team": true,
		"pushover API error: user already exists on the team":       true,
		"pushover API error: team token is invalid":                 false,
		"pushover API error: already used password":                 false,
		"": false,
	}
	for msg, want := range cases {
		var err error
		if msg != "" {
			err = &testError{msg: msg}
		}
		if got := isAlreadyOnTeamError(err); got != want {
			t.Errorf("isAlreadyOnTeamError(%q)=%v, want %v", msg, got, want)
		}
	}
}

func TestIsTeamMembershipNotFoundError(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"pushover API error: user not found":                         true,
		"pushover API error: not a member of this team":              true,
		"pushover API error: no pending invitation":                  true,
		"pushover API error: team token is invalid":                  false,
		"pushover API error: at least one administrator must remain": false,
		"pushover API returned HTTP 404: 404 page not found":         false,
		"": false,
	}
	for msg, want := range cases {
		var err error
		if msg != "" {
			err = &testError{msg: msg}
		}
		if got := isTeamMembershipNotFoundError(err); got != want {
			t.Errorf("isTeamMembershipNotFoundError(%q)=%v, want %v", msg, got, want)
		}
	}
}

func TestTeamUserResource_SensitiveAttributes(t *testing.T) {
	t.Parallel()
	sch := teamUserSchema(t)
	for _, name := range []string{"token", "password", "id"} {
		attr, ok := sch.Attributes[name].(schema.StringAttribute)
		if !ok {
			t.Fatalf("%s is not a string attribute", name)
		}
		if !attr.Sensitive {
			t.Errorf("%s must be sensitive", name)
		}
	}
}

func TestTeamUserResource_Read_RemovesMissingMembership(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != "team-token" {
			t.Errorf("team listing token mismatch")
		}
		writeTeamJSON(w, http.StatusOK, `{
			"status": 1,
			"request": "req-1",
			"name": "ops",
			"users": [{"id":"other","name":"Other","email":"other@example.com","administrator":false}]
		}`)
	})
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, nil)

	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected state to be removed when the user is absent from the team listing")
	}
}

func TestTeamUserResource_Read_PendingInviteKeepsState(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusOK, `{
			"status": 1,
			"request": "req-pending",
			"name": "ops",
			"users": [{"id":"other","name":"Other","email":"other@example.com","administrator":true}]
		}`)
	})
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "team-token/ada@example.com"),
		"user_id": tftypes.NewValue(tftypes.String, nil),
		"email":   tftypes.NewValue(tftypes.String, "ada@example.com"),
		"name":    tftypes.NewValue(tftypes.String, "Ada Lovelace"),
	})

	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("pending invite (null user_id) must be treated as present on Read")
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if !got.UserID.IsNull() {
		t.Fatalf("expected user_id to remain null while invite is pending, got %#v", got.UserID)
	}
	if got.ID.ValueString() != "team-token/ada@example.com" {
		t.Fatalf("expected provisional id preserved, got %q", got.ID.ValueString())
	}
	if got.Name.ValueString() != "Ada Lovelace" {
		t.Fatalf("expected configured name preserved, got %q", got.Name.ValueString())
	}
}

func TestTeamUserResource_Read_PendingInviteAdoptsUserIDWhenJoined(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusOK, `{
			"status": 1,
			"request": "req-joined",
			"name": "ops",
			"users": [{"id":"user-id-1","name":"Ada Lovelace","email":"ada@example.com","administrator":false}]
		}`)
	})
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "team-token/ada@example.com"),
		"user_id": tftypes.NewValue(tftypes.String, nil),
		"email":   tftypes.NewValue(tftypes.String, "ada@example.com"),
	})

	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.UserID.ValueString() != "user-id-1" {
		t.Fatalf("expected user_id adopted, got %q", got.UserID.ValueString())
	}
	if got.ID.ValueString() != "team-token/user-id-1" {
		t.Fatalf("expected id updated to team_token/user_id, got %q", got.ID.ValueString())
	}
}

func TestTeamUserResource_Read_UserIDMissDoesNotAdoptEmail(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusOK, `{
			"status": 1,
			"request": "req-reuse",
			"name": "ops",
			"users": [{"id":"new-person","name":"Ada","email":"ada@example.com","administrator":false}]
		}`)
	})
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, map[string]tftypes.Value{
		"user_id": tftypes.NewValue(tftypes.String, "old-user-id"),
		"email":   tftypes.NewValue(tftypes.String, "ada@example.com"),
	})

	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("expected removal when the stored user id is gone, even if the email was reused")
	}
}

func TestTeamUserResource_Read_SyncsAdminAndNameDrift(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusOK, `{
			"status": 1,
			"request": "req-2",
			"name": "ops",
			"users": [{"id":"user-id-1","name":"Remote Name","email":"ada@example.com","administrator":true}]
		}`)
	})
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, map[string]tftypes.Value{
		"email":    tftypes.NewValue(tftypes.String, "Ada@Example.com"),
		"name":     tftypes.NewValue(tftypes.String, "Local Name"),
		"password": tftypes.NewValue(tftypes.String, "kept-password"),
		"instant":  tftypes.NewValue(tftypes.Bool, true),
		"admin":    tftypes.NewValue(tftypes.Bool, false),
		"group":    tftypes.NewValue(tftypes.String, "Ops"),
	})

	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if !got.Admin.ValueBool() {
		t.Error("expected admin=true after refresh")
	}
	if got.Name.ValueString() != "Remote Name" {
		t.Errorf("name: %q", got.Name.ValueString())
	}
	if got.Email.ValueString() != "Ada@Example.com" {
		t.Errorf("email case changed: %q", got.Email.ValueString())
	}
	if got.Password.ValueString() != "kept-password" || !got.Instant.ValueBool() || got.Group.ValueString() != "Ops" {
		t.Error("create-time attributes changed on refresh")
	}
	if got.ID.ValueString() != "team-token/user-id-1" {
		t.Errorf("id: %q", got.ID.ValueString())
	}
}

func TestTeamUserResource_Read_LeavesUnsetName(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusOK, `{
			"status": 1,
			"request": "req-3",
			"name": "ops",
			"users": [{"id":"user-id-1","name":"Remote Name","email":"ada@example.com","administrator":false}]
		}`)
	})
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, nil)
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if !got.Name.IsNull() {
		t.Fatalf("expected name to stay null, got %#v", got.Name)
	}
}

func TestTeamUserResource_Read_PopulatesEmailFromUserID(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusOK, `{
			"status": 1,
			"request": "req-import",
			"name": "ops",
			"users": [{"id":"user-id-1","name":"Ada","email":"ada@example.com","administrator":true}]
		}`)
	})
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, map[string]tftypes.Value{
		"email": tftypes.NewValue(tftypes.String, nil),
		"admin": tftypes.NewValue(tftypes.Bool, false),
	})
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.Email.ValueString() != "ada@example.com" {
		t.Errorf("email: %q", got.Email.ValueString())
	}
	if !got.Admin.ValueBool() {
		t.Error("expected administrator flag from the listing")
	}
}

func TestTeamUserResource_Read_InvalidTokenKeepsState(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusBadRequest, `{"status":0,"errors":["team token is invalid"],"request":"r"}`)
	})
	assertTeamReadKeepsStateWithError(t, res)
}

func TestTeamUserResource_Read_UnrelatedHTTP404KeepsState(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404 page not found"))
	})
	assertTeamReadKeepsStateWithError(t, res)
}

func TestTeamUserResource_Create_PostsFieldsAndStoresUserID(t *testing.T) {
	t.Parallel()

	var addCalled, listCalled bool
	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			addCalled = true
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				return
			}
			if r.FormValue("token") != "team-token" || r.FormValue("email") != "ada@example.com" {
				t.Errorf("identity fields mismatch")
			}
			if r.FormValue("name") != "Ada Lovelace" || r.FormValue("password") != "not-a-real-password" {
				t.Errorf("name or password mismatch")
			}
			if r.FormValue("instant") != "true" || r.FormValue("admin") != "true" || r.FormValue("group") != "Ops" {
				t.Errorf("flag or group mismatch")
			}
			writeTeamJSON(w, http.StatusOK, `{"status":1,"request":"req-add"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			listCalled = true
			if r.URL.Query().Get("token") != "team-token" {
				t.Errorf("listing used the wrong token")
			}
			writeTeamJSON(w, http.StatusOK, `{
				"status":1,"request":"req-list","name":"ops",
				"users":[{"id":"user-id-1","name":"Ada Lovelace","email":"ada@example.com","administrator":true}]
			}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			writeTeamJSON(w, http.StatusNotFound, `{"status":0,"errors":["not found"]}`)
		}
	})

	sch := teamUserSchema(t)
	plan := tfsdk.Plan{
		Schema: sch,
		Raw: teamUserObject(t, sch, map[string]tftypes.Value{
			"id":       tftypes.NewValue(tftypes.String, nil),
			"name":     tftypes.NewValue(tftypes.String, "Ada Lovelace"),
			"password": tftypes.NewValue(tftypes.String, "not-a-real-password"),
			"instant":  tftypes.NewValue(tftypes.Bool, true),
			"admin":    tftypes.NewValue(tftypes.Bool, true),
			"group":    tftypes.NewValue(tftypes.String, "Ops"),
			"user_id":  tftypes.NewValue(tftypes.String, nil),
		}),
	}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	res.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !addCalled || !listCalled {
		t.Fatalf("add=%v list=%v", addCalled, listCalled)
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.UserID.ValueString() != "user-id-1" || got.ID.ValueString() != "team-token/user-id-1" {
		t.Fatalf("id=%q user_id=%q", got.ID.ValueString(), got.UserID.ValueString())
	}
	if got.Password.ValueString() != "not-a-real-password" {
		t.Fatal("password was not stored")
	}
}

func TestTeamUserResource_Create_OmitsOptionalFields(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				return
			}
			for _, key := range []string{"name", "password", "instant", "admin", "group"} {
				if _, ok := r.Form[key]; ok {
					t.Errorf("optional field %s was sent", key)
				}
			}
			writeTeamJSON(w, http.StatusOK, `{"status":1,"request":"req-add"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			writeTeamJSON(w, http.StatusOK, `{
				"status":1,"request":"req-list","name":"ops",
				"users":[{"id":"user-id-1","name":"","email":"ada@example.com","administrator":false}]
			}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	sch := teamUserSchema(t)
	plan := tfsdk.Plan{Schema: sch, Raw: teamUserObject(t, sch, nil)}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	res.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}

func TestTeamUserResource_Create_AlreadyOnTeamReadsListing(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			writeTeamJSON(w, http.StatusBadRequest, `{"status":0,"errors":["user is already a member of this team"],"request":"dup"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			writeTeamJSON(w, http.StatusOK, `{
				"status":1,"request":"req-list","name":"ops",
				"users":[{"id":"user-id-1","name":"Ada","email":"ada@example.com","administrator":false}]
			}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	sch := teamUserSchema(t)
	plan := tfsdk.Plan{Schema: sch, Raw: teamUserObject(t, sch, nil)}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	res.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.UserID.ValueString() != "user-id-1" {
		t.Fatalf("user id: %q", got.UserID.ValueString())
	}
}

func TestTeamUserResource_Create_ListingMissKeepsProvisionalID(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			writeTeamJSON(w, http.StatusOK, `{"status":1,"request":"req-add"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			writeTeamJSON(w, http.StatusOK, `{"status":1,"request":"req-list","name":"ops","users":[]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	sch := teamUserSchema(t)
	plan := tfsdk.Plan{Schema: sch, Raw: teamUserObject(t, sch, nil)}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: sch}}
	res.Create(context.Background(), resource.CreateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if !got.UserID.IsNull() {
		t.Fatalf("expected null user id, got %#v", got.UserID)
	}
	if got.ID.ValueString() != "team-token/ada@example.com" {
		t.Fatalf("id: %q", got.ID.ValueString())
	}
}

func TestTeamUserResource_Delete_RemoveUser(t *testing.T) {
	t.Parallel()

	var removeCalled, revokeCalled bool
	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
			return
		}
		if r.FormValue("token") != "team-token" || r.FormValue("email") != "ada@example.com" {
			t.Errorf("identity fields mismatch")
		}
		switch r.URL.Path {
		case "/teams/remove_user.json":
			removeCalled = true
			writeTeamJSON(w, http.StatusOK, `{"status":1,"request":"req-del"}`)
		case "/teams/revoke_invitation.json":
			revokeCalled = true
			writeTeamJSON(w, http.StatusOK, `{"status":1,"request":"req-revoke"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	callTeamDelete(t, res)
	if !removeCalled {
		t.Fatal("expected remove_user")
	}
	if revokeCalled {
		t.Fatal("revoke must not run after a successful remove")
	}
}

func TestTeamUserResource_Delete_RevokesPendingInvitation(t *testing.T) {
	t.Parallel()

	var revokeCalled bool
	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/teams/remove_user.json":
			writeTeamJSON(w, http.StatusBadRequest, `{"status":0,"errors":["user not found"],"request":"miss"}`)
		case "/teams/revoke_invitation.json":
			revokeCalled = true
			writeTeamJSON(w, http.StatusOK, `{"status":1,"request":"req-revoke"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	resp := callTeamDelete(t, res)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if !revokeCalled {
		t.Fatal("expected revoke_invitation when remove reports the user is absent")
	}
}

func TestTeamUserResource_Delete_AlreadyGone(t *testing.T) {
	t.Parallel()

	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		writeTeamJSON(w, http.StatusBadRequest, `{"status":0,"errors":["no pending invitation"],"request":"gone"}`)
	})
	resp := callTeamDelete(t, res)
	if resp.Diagnostics.HasError() {
		t.Fatalf("absent user should delete cleanly, got %v", resp.Diagnostics)
	}
}

func TestTeamUserResource_Delete_AdminConstraintFails(t *testing.T) {
	t.Parallel()

	var revokeCalled bool
	res := newTeamUserResource(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/teams/revoke_invitation.json" {
			revokeCalled = true
		}
		writeTeamJSON(w, http.StatusBadRequest, `{"status":0,"errors":["at least one administrator must remain"],"request":"admin"}`)
	})
	resp := callTeamDelete(t, res)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the administrator constraint to fail delete")
	}
	if revokeCalled {
		t.Fatal("revoke must not run when remove fails for a reason other than absence")
	}
}

func TestTeamUserResource_ImportState(t *testing.T) {
	t.Parallel()

	res := &TeamUserResource{}
	sch := teamUserSchema(t)
	resp := &resource.ImportStateResponse{State: nullTeamState(sch)}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "team-token/user-id-1"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	var got TeamUserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if got.Token.ValueString() != "team-token" || got.UserID.ValueString() != "user-id-1" {
		t.Fatalf("token/user_id not imported: token=%q user_id=%q", got.Token.ValueString(), got.UserID.ValueString())
	}
	if !got.Email.IsNull() {
		t.Fatal("email should be filled by Read, not ImportState")
	}
	if got.ID.ValueString() != "team-token/user-id-1" {
		t.Fatalf("id: %q", got.ID.ValueString())
	}
	if got.Admin.ValueBool() || got.Instant.ValueBool() {
		t.Fatal("expected admin and instant defaults")
	}
	if !got.Password.IsNull() {
		t.Fatal("expected password to be null after import")
	}

	emailResp := &resource.ImportStateResponse{State: nullTeamState(sch)}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "team-token/ada@example.com"}, emailResp)
	if emailResp.Diagnostics.HasError() {
		t.Fatalf("email import: %v", emailResp.Diagnostics)
	}
	var emailState TeamUserResourceModel
	if diags := emailResp.State.Get(context.Background(), &emailState); diags.HasError() {
		t.Fatalf("state get: %v", diags)
	}
	if emailState.Email.ValueString() != "ada@example.com" || !emailState.UserID.IsNull() {
		t.Fatalf("email import state: email=%q user_id null=%v", emailState.Email.ValueString(), emailState.UserID.IsNull())
	}
}

func TestTeamUserResource_ImportState_InvalidID(t *testing.T) {
	t.Parallel()
	res := &TeamUserResource{}
	sch := teamUserSchema(t)
	resp := &resource.ImportStateResponse{State: nullTeamState(sch)}
	res.ImportState(context.Background(), resource.ImportStateRequest{ID: "only-token"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid import id diagnostic")
	}
}

func nullTeamState(sch schema.Schema) tfsdk.State {
	return tfsdk.State{
		Schema: sch,
		Raw:    tftypes.NewValue(sch.Type().TerraformType(context.Background()), nil),
	}
}

func teamUserSchema(t *testing.T) schema.Schema {
	t.Helper()
	res := &TeamUserResource{}
	resp := &resource.SchemaResponse{}
	res.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	return resp.Schema
}

func newTeamUserResource(t *testing.T, h http.HandlerFunc) *TeamUserResource {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &TeamUserResource{
		client: pushover.NewClientWithBase("app-token", srv.URL, srv.Client()),
	}
}

func writeTeamJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func teamUserObject(t *testing.T, sch schema.Schema, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()
	vals := map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, nil),
		"token":    tftypes.NewValue(tftypes.String, "team-token"),
		"email":    tftypes.NewValue(tftypes.String, "ada@example.com"),
		"name":     tftypes.NewValue(tftypes.String, nil),
		"password": tftypes.NewValue(tftypes.String, nil),
		"instant":  tftypes.NewValue(tftypes.Bool, false),
		"admin":    tftypes.NewValue(tftypes.Bool, false),
		"group":    tftypes.NewValue(tftypes.String, nil),
		"user_id":  tftypes.NewValue(tftypes.String, nil),
	}
	for k, v := range overrides {
		vals[k] = v
	}
	return tftypes.NewValue(sch.Type().TerraformType(context.Background()), vals)
}

func teamUserRawState(t *testing.T, sch schema.Schema, overrides map[string]tftypes.Value) tfsdk.State {
	t.Helper()
	vals := map[string]tftypes.Value{
		"id":      tftypes.NewValue(tftypes.String, "team-token/user-id-1"),
		"user_id": tftypes.NewValue(tftypes.String, "user-id-1"),
	}
	return tfsdk.State{Schema: sch, Raw: teamUserObject(t, sch, mergeTeamValues(vals, overrides))}
}

func mergeTeamValues(base, overrides map[string]tftypes.Value) map[string]tftypes.Value {
	if overrides == nil {
		return base
	}
	out := make(map[string]tftypes.Value, len(base)+len(overrides))
	for k, v := range overrides {
		out[k] = v
	}
	for k, v := range base {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func assertTeamReadKeepsStateWithError(t *testing.T, res *TeamUserResource) {
	t.Helper()
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, nil)
	resp := resource.ReadResponse{State: state}
	res.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a diagnostic")
	}
	if resp.State.Raw.IsNull() {
		t.Fatal("expected state to be kept")
	}
	joined := resp.Diagnostics.Errors()
	for _, d := range joined {
		if strings.Contains(d.Detail(), "team-token") || strings.Contains(d.Summary(), "team-token") {
			t.Fatal("diagnostic includes the team token")
		}
	}
}

func callTeamDelete(t *testing.T, res *TeamUserResource) resource.DeleteResponse {
	t.Helper()
	sch := teamUserSchema(t)
	state := teamUserRawState(t, sch, nil)
	resp := resource.DeleteResponse{State: state}
	res.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
	return resp
}
