// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/provider"
)

// Lifecycle contract tests drive create, read, import, drift, and destroy for
// every registered resource through Terraform (or OpenTofu, when
// TF_ACC_TERRAFORM_PATH points at tofu). The HTTP API is an httptest double.
// Nothing here calls Pushover and nothing requires TF_ACC or credentials.
//
// OpenTofu 1.11 rejects the legacy "-" provider namespace on
// registry.terraform.io. When the selected binary's name starts with "tofu",
// the suite sets TF_ACC_PROVIDER_HOST=registry.opentofu.org unless that
// variable is already set. The plugin-testing harness then registers both
// the "-" and "hashicorp" namespaces on a host OpenTofu accepts.
//
// Resources whose API cannot express a phase say so in the test and in
// docs/lifecycle-contract.md. Those cases still run: import is attempted and
// must fail, and a no-op read must not invent a diff.

const (
	lifecycleAppToken  = "lifecycle-test-token"
	lifecycleUserKey   = "uLifecycleUser00000000000001"
	lifecycleGroupKey  = "gLifecycle"
	lifecycleTeamToken = "team-token-lifecycle"
	lifecycleReceipt   = "rcpt-lifecycle"
	lifecycleSubKey    = "uSubLifecycle000000000000000001"
	lifecycleTeamUser  = "uAdaLovelace000000000000000001"
)

const lifecycleProviderHCL = `
provider "pushover" {
  api_token = "lifecycle-test-token"
}
`

func TestLifecycleContract(t *testing.T) {
	cases := map[string]func(*testing.T){
		"pushover_glances":                 testLifecycleGlances,
		"pushover_group":                   testLifecycleGroup,
		"pushover_group_user":              testLifecycleGroupUser,
		"pushover_license_assignment":      testLifecycleLicenseAssignment,
		"pushover_message":                 testLifecycleMessage,
		"pushover_receipt":                 testLifecycleReceipt,
		"pushover_subscription_subscriber": testLifecycleSubscriptionSubscriber,
		"pushover_team_user":               testLifecycleTeamUser,
	}

	seen := map[string]bool{}
	for _, newRes := range provider.New("test")().Resources(context.Background()) {
		res := newRes()
		var meta fwresource.MetadataResponse
		res.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "pushover"}, &meta)
		seen[meta.TypeName] = true
		run, ok := cases[meta.TypeName]
		if !ok {
			t.Errorf("no lifecycle contract for %s", meta.TypeName)
			continue
		}
		t.Run(meta.TypeName, run)
	}
	for name := range cases {
		if !seen[name] {
			t.Errorf("lifecycle contract lists unregistered resource %s", name)
		}
	}
}

func TestLifecycleContractSensitiveSchema(t *testing.T) {
	want := map[string][]string{
		"pushover_glances":                 {"api_token"},
		"pushover_group":                   nil,
		"pushover_group_user":              nil,
		"pushover_license_assignment":      {"api_token"},
		"pushover_message":                 {"api_token"},
		"pushover_receipt":                 {"api_token"},
		"pushover_subscription_subscriber": {"api_token"},
		"pushover_team_user":               {"id", "password", "token"},
	}
	for name, paths := range want {
		sort.Strings(paths)
		want[name] = paths
	}

	seen := map[string]bool{}
	for _, newRes := range provider.New("test")().Resources(context.Background()) {
		res := newRes()
		var meta fwresource.MetadataResponse
		res.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "pushover"}, &meta)
		seen[meta.TypeName] = true
		got := lifecycleSensitivePaths(t, res)
		expected, ok := want[meta.TypeName]
		if !ok {
			t.Errorf("sensitive schema does not list %s (got %#v)", meta.TypeName, got)
			continue
		}
		if !slices.Equal(got, expected) {
			t.Errorf("%s sensitive paths = %#v, want %#v", meta.TypeName, got, expected)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("sensitive schema lists unregistered resource %s", name)
		}
	}
}

func testLifecycleMessage(t *testing.T) {
	t.Helper()
	var messages, cancels int
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/messages.json":
			mustFormToken(t, r, lifecycleAppToken)
			if r.FormValue("user") != lifecycleUserKey || r.FormValue("message") != "lifecycle contract" {
				t.Errorf("message form user=%q message=%q", r.FormValue("user"), r.FormValue("message"))
			}
			if r.FormValue("priority") != "2" || r.FormValue("retry") != "60" || r.FormValue("expire") != "300" {
				t.Errorf("emergency form priority=%q retry=%q expire=%q", r.FormValue("priority"), r.FormValue("retry"), r.FormValue("expire"))
			}
			messages++
			writeAPI(w, `{"status":1,"request":"req-message","receipt":"`+lifecycleReceipt+`"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/receipts/"+lifecycleReceipt+"/cancel.json":
			mustFormToken(t, r, lifecycleAppToken)
			cancels++
			writeAPI(w, `{"status":1,"request":"req-cancel"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_message" "alert" {
  user_key          = "` + lifecycleUserKey + `"
  message           = "lifecycle contract"
  priority          = 2
  retry             = 60
  expire            = 300
  cancel_on_destroy = true
}
`
	drift := lifecycleProviderHCL + `
resource "pushover_message" "alert" {
  user_key          = "` + lifecycleUserKey + `"
  message           = "lifecycle contract drifted"
  priority          = 2
  retry             = 60
  expire            = 300
  cancel_on_destroy = true
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_message.alert", "receipt", lifecycleReceipt),
				resource.TestCheckResourceAttr("pushover_message.alert", "request_id", "req-message"),
				resource.TestCheckResourceAttr("pushover_message.alert", "cancel_on_destroy", "true"),
			),
		},
		{
			// Read is a no-op. An unchanged config must not plan another send.
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		unsupportedImportStep("pushover_message.alert"),
		{
			// Sent messages cannot be read back, so remote drift is invisible.
			// A configuration change is the drift signal, and it forces replacement.
			// Plan only: do not send a second message.
			Config:             drift,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
	})
	if messages != 1 {
		t.Fatalf("messages.json calls = %d, want 1", messages)
	}
	if cancels != 1 {
		t.Fatalf("receipt cancel calls = %d, want 1 (destroy cancels the emergency receipt)", cancels)
	}
}

func testLifecycleReceipt(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	delivered := int64(10)
	var gets, cancels int
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/receipts/"+lifecycleReceipt+".json":
			if r.URL.Query().Get("token") != lifecycleAppToken {
				t.Errorf("receipt token = %q", r.URL.Query().Get("token"))
			}
			mu.Lock()
			last := delivered
			mu.Unlock()
			gets++
			writeAPI(w, `{"status":1,"request":"req-receipt","acknowledged":0,"acknowledged_at":0,"acknowledged_by":"","acknowledged_by_device":"","last_delivered_at":`+itoa(last)+`,"expired":0,"expires_at":4102444800,"called_back":0,"called_back_at":0}`)
		case r.Method == http.MethodPost && r.URL.Path == "/receipts/"+lifecycleReceipt+"/cancel.json":
			mustFormToken(t, r, lifecycleAppToken)
			cancels++
			writeAPI(w, `{"status":1,"request":"req-cancel"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_receipt" "outage" {
  receipt           = "` + lifecycleReceipt + `"
  cancel_on_destroy = true
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_receipt.outage", "id", lifecycleReceipt),
				resource.TestCheckResourceAttr("pushover_receipt.outage", "acknowledged", "false"),
				resource.TestCheckResourceAttr("pushover_receipt.outage", "expired", "false"),
				resource.TestCheckResourceAttr("pushover_receipt.outage", "last_delivered_at", "10"),
				resource.TestCheckResourceAttr("pushover_receipt.outage", "request_id", "req-receipt"),
			),
		},
		{
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		{
			ResourceName:            "pushover_receipt.outage",
			ImportState:             true,
			ImportStateId:           lifecycleReceipt,
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: []string{"cancel_on_destroy"},
		},
		{
			// Computed status moves with the API. That refresh must not plan a change.
			PreConfig: func() {
				mu.Lock()
				delivered = 20
				mu.Unlock()
			},
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_receipt.outage", "last_delivered_at", "20"),
				resource.TestCheckResourceAttr("pushover_receipt.outage", "acknowledged", "false"),
			),
		},
	})
	if gets < 2 {
		t.Fatalf("receipt reads = %d, want at least create and refresh", gets)
	}
	if cancels != 1 {
		t.Fatalf("receipt cancel calls = %d, want 1", cancels)
	}
}

func testLifecycleGroup(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	name := "Ops"
	var renames int
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/groups/"+lifecycleGroupKey+"/rename.json":
			mustFormToken(t, r, lifecycleAppToken)
			mu.Lock()
			name = r.FormValue("name")
			mu.Unlock()
			renames++
			writeAPI(w, `{"status":1,"request":"req-rename"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/groups/"+lifecycleGroupKey+".json":
			if r.URL.Query().Get("token") != lifecycleAppToken {
				t.Errorf("group token = %q", r.URL.Query().Get("token"))
			}
			mu.Lock()
			current := name
			mu.Unlock()
			writeAPI(w, `{"status":1,"request":"req-group","name":"`+current+`","users":[]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_group" "ops" {
  group_key = "` + lifecycleGroupKey + `"
  name      = "Ops"
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_group.ops", "id", lifecycleGroupKey),
				resource.TestCheckResourceAttr("pushover_group.ops", "name", "Ops"),
			),
		},
		{
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		{
			ResourceName:      "pushover_group.ops",
			ImportState:       true,
			ImportStateId:     lifecycleGroupKey,
			ImportStateVerify: true,
		},
		{
			PreConfig: func() {
				mu.Lock()
				name = "Drifted"
				mu.Unlock()
			},
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		{
			// Apply renames the group back to the configured name.
			Config: cfg,
			Check:  resource.TestCheckResourceAttr("pushover_group.ops", "name", "Ops"),
		},
	})
	if renames != 2 {
		t.Fatalf("rename calls = %d, want 2 (create and drift repair)", renames)
	}
}

func testLifecycleGroupUser(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	memo := "on call"
	var adds, deletes int
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/groups/"+lifecycleGroupKey+"/add_user.json":
			mustFormToken(t, r, lifecycleAppToken)
			if r.FormValue("user") != lifecycleUserKey {
				t.Errorf("add_user user = %q", r.FormValue("user"))
			}
			mu.Lock()
			memo = r.FormValue("memo")
			mu.Unlock()
			adds++
			writeAPI(w, `{"status":1,"request":"req-add-user"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/groups/"+lifecycleGroupKey+".json":
			if r.URL.Query().Get("token") != lifecycleAppToken {
				t.Errorf("group token = %q", r.URL.Query().Get("token"))
			}
			mu.Lock()
			current := memo
			mu.Unlock()
			writeAPI(w, `{"status":1,"request":"req-group","name":"Ops","users":[{"user":"`+lifecycleUserKey+`","device":"","memo":"`+current+`","disabled":false}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/groups/"+lifecycleGroupKey+"/delete_user.json":
			mustFormToken(t, r, lifecycleAppToken)
			if r.FormValue("user") != lifecycleUserKey {
				t.Errorf("delete_user user = %q", r.FormValue("user"))
			}
			deletes++
			writeAPI(w, `{"status":1,"request":"req-delete-user"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_group_user" "member" {
  group_key = "` + lifecycleGroupKey + `"
  user_key  = "` + lifecycleUserKey + `"
  memo      = "on call"
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_group_user.member", "id", lifecycleGroupKey+"/"+lifecycleUserKey),
				resource.TestCheckResourceAttr("pushover_group_user.member", "memo", "on call"),
				resource.TestCheckResourceAttr("pushover_group_user.member", "disabled", "false"),
			),
		},
		{
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		{
			ResourceName:      "pushover_group_user.member",
			ImportState:       true,
			ImportStateId:     lifecycleGroupKey + "/" + lifecycleUserKey,
			ImportStateVerify: true,
		},
		{
			PreConfig: func() {
				mu.Lock()
				memo = "drifted"
				mu.Unlock()
			},
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
		{
			Config: cfg,
			Check:  resource.TestCheckResourceAttr("pushover_group_user.member", "memo", "on call"),
		},
	})
	if adds != 2 {
		t.Fatalf("add_user calls = %d, want 2 (create and memo repair)", adds)
	}
	if deletes != 1 {
		t.Fatalf("delete_user calls = %d, want 1", deletes)
	}
}

func testLifecycleTeamUser(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	admin := false
	var adds, removes, revokes int
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/teams/add_user.json":
			mustFormToken(t, r, lifecycleTeamToken)
			if r.FormValue("email") != "ada@example.com" {
				t.Errorf("add_user email = %q", r.FormValue("email"))
			}
			adds++
			writeAPI(w, `{"status":1,"request":"req-team-add"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/teams.json":
			if r.URL.Query().Get("token") != lifecycleTeamToken {
				t.Errorf("teams token = %q", r.URL.Query().Get("token"))
			}
			mu.Lock()
			isAdmin := admin
			mu.Unlock()
			adminJSON := "false"
			if isAdmin {
				adminJSON = "true"
			}
			writeAPI(w, `{"status":1,"request":"req-team","name":"Lifecycle","users":[{"id":"`+lifecycleTeamUser+`","name":"Ada Lovelace","email":"ada@example.com","administrator":`+adminJSON+`}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/teams/remove_user.json":
			mustFormToken(t, r, lifecycleTeamToken)
			removes++
			writeAPI(w, `{"status":1,"request":"req-team-remove"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/teams/revoke_invitation.json":
			revokes++
			writeAPI(w, `{"status":1,"request":"req-team-revoke"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_team_user" "ada" {
  token = "` + lifecycleTeamToken + `"
  email = "ada@example.com"
  admin = false
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_team_user.ada", "id", lifecycleTeamToken+"/"+lifecycleTeamUser),
				resource.TestCheckResourceAttr("pushover_team_user.ada", "user_id", lifecycleTeamUser),
				resource.TestCheckResourceAttr("pushover_team_user.ada", "admin", "false"),
				resource.TestCheckResourceAttr("pushover_team_user.ada", "email", "ada@example.com"),
			),
		},
		{
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		{
			// Import does not restore name or password. Name stays null because Read
			// will not adopt a remote name the configuration was not managing, and
			// the Teams API never returns a password.
			ResourceName:            "pushover_team_user.ada",
			ImportState:             true,
			ImportStateId:           lifecycleTeamToken + "/" + lifecycleTeamUser,
			ImportStateVerify:       true,
			ImportStateVerifyIgnore: []string{"name", "password"},
		},
		{
			// Admin is configuration, and the Teams API has no update call.
			// Remote drift plans a replacement. Do not apply it.
			PreConfig: func() {
				mu.Lock()
				admin = true
				mu.Unlock()
			},
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
	})
	if adds != 1 {
		t.Fatalf("add_user calls = %d, want 1", adds)
	}
	if removes != 1 {
		t.Fatalf("remove_user calls = %d, want 1", removes)
	}
	if revokes != 0 {
		t.Fatalf("revoke_invitation calls = %d, want 0 when remove_user succeeds", revokes)
	}
}

func testLifecycleSubscriptionSubscriber(t *testing.T) {
	t.Helper()
	var mu sync.Mutex
	gone := false
	var migrates, validates int
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/subscriptions/migrate.json":
			mustFormToken(t, r, lifecycleAppToken)
			if r.FormValue("subscription") != "lifecycle-sub" || r.FormValue("user") != lifecycleUserKey {
				t.Errorf("migrate subscription=%q user=%q", r.FormValue("subscription"), r.FormValue("user"))
			}
			migrates++
			writeAPI(w, `{"status":1,"request":"req-migrate","subscribed_user_key":"`+lifecycleSubKey+`"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/users/validate.json":
			mustFormToken(t, r, lifecycleAppToken)
			if r.FormValue("user") != lifecycleSubKey {
				t.Errorf("validate user = %q", r.FormValue("user"))
			}
			validates++
			mu.Lock()
			missing := gone
			mu.Unlock()
			if missing {
				writeAPI(w, `{"status":0,"request":"req-validate","errors":["user key is invalid"]}`)
				return
			}
			writeAPI(w, `{"status":1,"request":"req-validate","group":0,"devices":["phone"]}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_subscription_subscriber" "member" {
  subscription = "lifecycle-sub"
  user_key     = "` + lifecycleUserKey + `"
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_subscription_subscriber.member", "id", "lifecycle-sub/"+lifecycleUserKey),
				resource.TestCheckResourceAttr("pushover_subscription_subscriber.member", "subscribed_user_key", lifecycleSubKey),
			),
		},
		{
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		unsupportedImportStep("pushover_subscription_subscriber.member"),
		{
			// The subscriber key is gone. Refresh drops state and the plan recreates.
			// Plan only, so migrate is not called again.
			PreConfig: func() {
				mu.Lock()
				gone = true
				mu.Unlock()
			},
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
	})
	if migrates != 1 {
		t.Fatalf("migrate calls = %d, want 1", migrates)
	}
	if validates < 1 {
		t.Fatalf("validate calls = %d, want at least 1", validates)
	}
}

func testLifecycleLicenseAssignment(t *testing.T) {
	t.Helper()
	var assigns int
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/licenses/assign.json":
			mustFormToken(t, r, lifecycleAppToken)
			if r.FormValue("user") != lifecycleUserKey || r.FormValue("os") != "Android" {
				t.Errorf("assign user=%q os=%q", r.FormValue("user"), r.FormValue("os"))
			}
			assigns++
			writeAPI(w, `{"status":1,"request":"req-license","credits":4}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_license_assignment" "credit" {
  user_key = "` + lifecycleUserKey + `"
  os       = "Android"
}
`
	// A replacement would spend another credit. Plan only.
	drift := lifecycleProviderHCL + `
resource "pushover_license_assignment" "credit" {
  user_key = "` + lifecycleUserKey + `"
  os       = "iOS"
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_license_assignment.credit", "id", "user:"+lifecycleUserKey+"/Android"),
				resource.TestCheckResourceAttr("pushover_license_assignment.credit", "credits", "4"),
				resource.TestCheckResourceAttr("pushover_license_assignment.credit", "request_id", "req-license"),
			),
		},
		{
			// Read never calls the assignment endpoint.
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		unsupportedImportStep("pushover_license_assignment.credit"),
		{
			Config:             drift,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
	})
	if assigns != 1 {
		t.Fatalf("assign calls = %d, want 1 (create only; refresh, plan, and destroy must not assign)", assigns)
	}
}

func testLifecycleGlances(t *testing.T) {
	t.Helper()
	var posts, clears int
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/glances.json" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		mustFormToken(t, r, lifecycleAppToken)
		if r.FormValue("user") != lifecycleUserKey {
			t.Errorf("glance user = %q", r.FormValue("user"))
		}
		posts++
		if _, ok := r.Form["text"]; ok && r.FormValue("text") == "" {
			clears++
		}
		writeAPI(w, `{"status":1,"request":"req-glance"}`)
	}

	cfg := lifecycleProviderHCL + `
resource "pushover_glances" "widget" {
  user_key = "` + lifecycleUserKey + `"
  text     = "ok"
}
`
	drift := lifecycleProviderHCL + `
resource "pushover_glances" "widget" {
  user_key = "` + lifecycleUserKey + `"
  text     = "drift"
}
`
	runLifecycle(t, handler, []resource.TestStep{
		{
			Config: cfg,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("pushover_glances.widget", "id", lifecycleUserKey),
				resource.TestCheckResourceAttr("pushover_glances.widget", "text", "ok"),
				resource.TestCheckResourceAttr("pushover_glances.widget", "request_id", "req-glance"),
			),
		},
		{
			// Read is a no-op, so an unchanged config does not push again.
			Config:             cfg,
			PlanOnly:           true,
			ExpectNonEmptyPlan: false,
		},
		unsupportedImportStep("pushover_glances.widget"),
		{
			// Glance data has no read API. Configuration drift plans an in-place update.
			// Plan only, so the drifted text is not pushed.
			Config:             drift,
			PlanOnly:           true,
			ExpectNonEmptyPlan: true,
		},
	})
	if posts != 2 {
		t.Fatalf("glances posts = %d, want 2 (create and destroy clear)", posts)
	}
	if clears != 1 {
		t.Fatalf("glances clear posts = %d, want 1", clears)
	}
}

func runLifecycle(t *testing.T, handler http.HandlerFunc, steps []resource.TestStep) {
	t.Helper()
	useOpenTofuRegistryHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Copy the body so a handler can parse the form once.
		if r.Body != nil && r.Method != http.MethodGet {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				http.Error(w, "read", http.StatusBadRequest)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytesNew(body))
			r.ContentLength = int64(len(body))
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("PUSHOVER_API_BASE_URL", srv.URL)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps:                    steps,
	})
}

// useOpenTofuRegistryHost makes the plugin-testing reattach map valid for
// OpenTofu 1.11. Terraform keeps the default registry.terraform.io host.
func useOpenTofuRegistryHost(t *testing.T) {
	t.Helper()
	bin := os.Getenv("TF_ACC_TERRAFORM_PATH")
	base := filepath.Base(bin)
	if bin == "" || (base != "tofu" && !strings.HasPrefix(base, "tofu")) {
		return
	}
	if os.Getenv("TF_ACC_PROVIDER_HOST") == "" {
		t.Setenv("TF_ACC_PROVIDER_HOST", "registry.opentofu.org")
	}
}

func unsupportedImportStep(resourceName string) resource.TestStep {
	return resource.TestStep{
		ResourceName:  resourceName,
		ImportState:   true,
		ImportStateId: "not-supported",
		ExpectError:   regexp.MustCompile(`(?i)import`),
	}
}

func mustFormToken(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Errorf("ParseForm: %v", err)
		return
	}
	if r.FormValue("token") != want {
		t.Errorf("%s %s token = %q, want %q", r.Method, r.URL.Path, r.FormValue("token"), want)
	}
}

func writeAPI(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func bytesNew(b []byte) *sliceReader {
	return &sliceReader{b: b}
}

type sliceReader struct {
	b []byte
	i int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

func lifecycleSensitivePaths(t *testing.T, res fwresource.Resource) []string {
	t.Helper()
	var resp fwresource.SchemaResponse
	res.Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", resp.Diagnostics)
	}
	var paths []string
	for name, attr := range resp.Schema.Attributes {
		collectSensitiveAttribute(name, attr, &paths)
	}
	for name, block := range resp.Schema.Blocks {
		collectSensitiveBlock(name, block, &paths)
	}
	sort.Strings(paths)
	return paths
}

func collectSensitiveAttribute(name string, attr schema.Attribute, out *[]string) {
	if attr.IsSensitive() {
		*out = append(*out, name)
	}
	switch nested := attr.(type) {
	case schema.SingleNestedAttribute:
		for childName, child := range nested.Attributes {
			collectSensitiveAttribute(name+"."+childName, child, out)
		}
	case schema.ListNestedAttribute:
		for childName, child := range nested.NestedObject.Attributes {
			collectSensitiveAttribute(name+"."+childName, child, out)
		}
	case schema.SetNestedAttribute:
		for childName, child := range nested.NestedObject.Attributes {
			collectSensitiveAttribute(name+"."+childName, child, out)
		}
	case schema.MapNestedAttribute:
		for childName, child := range nested.NestedObject.Attributes {
			collectSensitiveAttribute(name+"."+childName, child, out)
		}
	}
}

func collectSensitiveBlock(name string, block schema.Block, out *[]string) {
	switch nested := block.(type) {
	case schema.SingleNestedBlock:
		for childName, child := range nested.Attributes {
			collectSensitiveAttribute(name+"."+childName, child, out)
		}
	case schema.ListNestedBlock:
		for childName, child := range nested.NestedObject.Attributes {
			collectSensitiveAttribute(name+"."+childName, child, out)
		}
	case schema.SetNestedBlock:
		for childName, child := range nested.NestedObject.Attributes {
			collectSensitiveAttribute(name+"."+childName, child, out)
		}
	}
}
