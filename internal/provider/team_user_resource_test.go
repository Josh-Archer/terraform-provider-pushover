// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func readTeamFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "teams", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

// TestTeamUserResource_BasicSchema validates the minimal required fields are accepted.
func TestTeamUserResource_BasicSchema(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "basic" {
  token = "team-token"
  email = "ada@example.com"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestTeamUserResource_AllOptionalFields validates the add-user arguments are accepted.
func TestTeamUserResource_AllOptionalFields(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "full" {
  token    = "team-token"
  email    = "ada@example.com"
  name     = "Ada Lovelace"
  password = "not-a-real-password"
  instant  = true
  admin    = true
  group    = "Ops"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestTeamUserResource_Defaults validates admin and instant default to false.
func TestTeamUserResource_Defaults(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "defaults" {
  token = "team-token"
  email = "ada@example.com"
}`,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.defaults", "admin", "false"),
					resource.TestCheckResourceAttr("pushover_team_user.defaults", "instant", "false"),
				),
			},
		},
	})
}

// TestTeamUserResource_RejectsBadEmail validates the email format check.
func TestTeamUserResource_RejectsBadEmail(t *testing.T) {
	t.Parallel()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "pushover" { api_token = "fake" }

resource "pushover_team_user" "bad" {
  token = "team-token"
  email = "not-an-email"
}`,
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`must be an email address`),
			},
		},
	})
}

// TestTeamUserResource_Fixture_PendingInviteNoDiffAndRevoke exercises the invite
// path against recorded JSON fixtures when the invited user has not yet accepted
// the invitation (absent from GET /teams.json). It verifies that the post-apply
// refresh and subsequent plans produce no diff (instead of re-adding on every
// apply) and that destroy revokes the pending invitation.
func TestTeamUserResource_Fixture_PendingInviteNoDiffAndRevoke(t *testing.T) {
	addSuccess := readTeamFixture(t, "add_user_success.json")
	pendingListing := readTeamFixture(t, "team_pending_invite.json")
	removeNotFound := readTeamFixture(t, "remove_user_not_found.json")
	revokeSuccess := readTeamFixture(t, "revoke_invitation_success.json")

	var addCalls, listCalls, removeCalls, revokeCalls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			atomic.AddInt32(&addCalls, 1)
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				return
			}
			if r.FormValue("token") != "team-token-fixture" || r.FormValue("email") != "ada@example.com" {
				t.Errorf("unexpected add_user identity fields: token=%q email=%q", r.FormValue("token"), r.FormValue("email"))
			}
			if r.FormValue("name") != "Ada Lovelace" || r.FormValue("group") != "Ops" || r.FormValue("instant") != "true" {
				t.Errorf("unexpected add_user optional fields: name=%q group=%q instant=%q", r.FormValue("name"), r.FormValue("group"), r.FormValue("instant"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(addSuccess)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			atomic.AddInt32(&listCalls, 1)
			if r.URL.Query().Get("token") != "team-token-fixture" {
				t.Errorf("unexpected teams.json token: %q", r.URL.Query().Get("token"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(pendingListing)
		case r.Method == http.MethodPost && r.URL.Path == "/teams/remove_user.json":
			atomic.AddInt32(&removeCalls, 1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(removeNotFound)
		case r.Method == http.MethodPost && r.URL.Path == "/teams/revoke_invitation.json":
			atomic.AddInt32(&revokeCalls, 1)
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				return
			}
			if r.FormValue("token") != "team-token-fixture" || r.FormValue("email") != "ada@example.com" {
				t.Errorf("unexpected revoke_invitation fields: token=%q email=%q", r.FormValue("token"), r.FormValue("email"))
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(revokeSuccess)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	cfg := `
provider "pushover" { api_token = "fake_app_token" }

resource "pushover_team_user" "ada" {
  token   = "team-token-fixture"
  email   = "ada@example.com"
  name    = "Ada Lovelace"
  instant = true
  group   = "Ops"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.ada", "id", "team-token-fixture/ada@example.com"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "email", "ada@example.com"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "name", "Ada Lovelace"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "instant", "true"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "admin", "false"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "group", "Ops"),
					resource.TestCheckNoResourceAttr("pushover_team_user.ada", "user_id"),
				),
			},
			{
				// Re-planning while the invite is still pending must settle with no diff
				// and must not re-invoke add_user.json.
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})

	if got := atomic.LoadInt32(&addCalls); got != 1 {
		t.Fatalf("expected add_user.json to be called exactly once, got %d", got)
	}
	if got := atomic.LoadInt32(&listCalls); got < 2 {
		t.Fatalf("expected teams.json to be called on create and refresh, got %d", got)
	}
	if got := atomic.LoadInt32(&removeCalls); got != 1 {
		t.Fatalf("expected remove_user.json to be attempted once on destroy, got %d", got)
	}
	if got := atomic.LoadInt32(&revokeCalls); got != 1 {
		t.Fatalf("expected revoke_invitation.json fallback once on destroy, got %d", got)
	}
}

// TestTeamUserResource_Fixture_JoinDriftAndRemove exercises the full lifecycle
// using recorded JSON fixtures:
//  1. Create starts as a pending invite (no diff).
//  2. User accepts invite and appears in GET /teams.json; refresh adopts user_id with no diff.
//  3. Remote drift on admin and name produces a non-empty replacement plan; apply restores config.
//  4. Out-of-band removal drops the confirmed member from state on refresh, producing a non-empty recreate plan; apply re-adds them.
//  5. Destroy removes the confirmed member via POST /teams/remove_user.json without calling revoke_invitation.json.
func TestTeamUserResource_Fixture_JoinDriftAndRemove(t *testing.T) {
	addSuccess := readTeamFixture(t, "add_user_success.json")
	pendingListing := readTeamFixture(t, "team_pending_invite.json")
	joinedListing := readTeamFixture(t, "team_member_joined.json")
	driftedListing := readTeamFixture(t, "team_member_drifted.json")
	removedListing := readTeamFixture(t, "team_member_removed.json")
	removeSuccess := readTeamFixture(t, "remove_user_success.json")

	var mu sync.Mutex
	currentListing := pendingListing
	setListing := func(fixture []byte) {
		mu.Lock()
		currentListing = fixture
		mu.Unlock()
	}

	var addCalls, removeCalls, revokeCalls int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			callNum := atomic.AddInt32(&addCalls, 1)
			// Call 1 leaves the user as a pending invitation; subsequent add calls
			// (replacement after drift and re-creation after removal) land in the listing.
			if callNum > 1 {
				setListing(joinedListing)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(addSuccess)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			mu.Lock()
			body := currentListing
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		case r.Method == http.MethodPost && r.URL.Path == "/teams/remove_user.json":
			atomic.AddInt32(&removeCalls, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(removeSuccess)
		case r.Method == http.MethodPost && r.URL.Path == "/teams/revoke_invitation.json":
			atomic.AddInt32(&revokeCalls, 1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(removeSuccess)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	cfg := `
provider "pushover" { api_token = "fake_app_token" }

resource "pushover_team_user" "ada" {
  token = "team-token-fixture"
  email = "ada@example.com"
  name  = "Ada Lovelace"
  admin = false
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Step 1: Invite created while pending.
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.ada", "id", "team-token-fixture/ada@example.com"),
					resource.TestCheckNoResourceAttr("pushover_team_user.ada", "user_id"),
				),
			},
			{
				// Step 2: Invitation is accepted; refresh adopts user_id with no diff.
				PreConfig: func() {
					setListing(joinedListing)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: false,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.ada", "user_id", "uAdaLovelace000000000000000001"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "id", "team-token-fixture/uAdaLovelace000000000000000001"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "name", "Ada Lovelace"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "admin", "false"),
				),
			},
			{
				// Step 3: Out-of-band drift on admin and name produces a non-empty plan.
				PreConfig: func() {
					setListing(driftedListing)
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// Step 4: Apply replaces the drifted membership and restores the configured values.
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.ada", "user_id", "uAdaLovelace000000000000000001"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "name", "Ada Lovelace"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "admin", "false"),
				),
			},
			{
				// Step 5: Out-of-band removal of the confirmed member drops state on refresh
				// and produces a non-empty plan to recreate the membership.
				PreConfig: func() {
					setListing(removedListing)
				},
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// Step 6: Apply recreates the removed membership.
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.ada", "user_id", "uAdaLovelace000000000000000001"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "id", "team-token-fixture/uAdaLovelace000000000000000001"),
				),
			},
		},
	})

	if got := atomic.LoadInt32(&addCalls); got != 3 {
		t.Fatalf("expected 3 add_user.json calls (initial + drift replace + recreate after removal), got %d", got)
	}
	if got := atomic.LoadInt32(&removeCalls); got != 2 {
		t.Fatalf("expected 2 remove_user.json calls (drift replace + final destroy), got %d", got)
	}
	if got := atomic.LoadInt32(&revokeCalls); got != 0 {
		t.Fatalf("expected 0 revoke_invitation.json calls when remove_user.json succeeds, got %d", got)
	}
}

// TestTeamUserResource_Fixture_PasswordPreservedFromConfigAndNullOnImport verifies
// that password (marked Sensitive) is sent to add_user.json on create, preserved
// from prior/configured state across refreshes so plans settle with no diff
// (since terraform-plugin-framework v1.13 does not support WriteOnly attributes),
// and left null when importing an existing member.
func TestTeamUserResource_Fixture_PasswordPreservedFromConfigAndNullOnImport(t *testing.T) {
	addSuccess := readTeamFixture(t, "add_user_success.json")
	joinedListing := readTeamFixture(t, "team_member_joined.json")
	removeSuccess := readTeamFixture(t, "remove_user_success.json")

	var gotPassword string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			if err := r.ParseForm(); err != nil {
				t.Errorf("ParseForm: %v", err)
				return
			}
			gotPassword = r.FormValue("password")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(addSuccess)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(joinedListing)
		case r.Method == http.MethodPost && r.URL.Path == "/teams/remove_user.json":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(removeSuccess)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	cfg := `
provider "pushover" { api_token = "fake_app_token" }

resource "pushover_team_user" "ada" {
  token    = "team-token-fixture"
  email    = "ada@example.com"
  name     = "Ada Lovelace"
  password = "fixture-initial-pass"
}
`

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("pushover_team_user.ada", "user_id", "uAdaLovelace000000000000000001"),
					resource.TestCheckResourceAttr("pushover_team_user.ada", "password", "fixture-initial-pass"),
				),
			},
			{
				Config:             cfg,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
			{
				ResourceName:            "pushover_team_user.ada",
				ImportState:             true,
				ImportStateId:           "team-token-fixture/uAdaLovelace000000000000000001",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"name", "password"},
			},
		},
	})

	if gotPassword != "fixture-initial-pass" {
		t.Fatalf("expected password forwarded to add_user.json, got %q", gotPassword)
	}
}

