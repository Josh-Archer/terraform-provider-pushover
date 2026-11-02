// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// emailPattern is a practical check, not a full RFC parser. Pushover identifies
// team members by email address on add and remove.
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &TeamUserResource{}
	_ resource.ResourceWithImportState = &TeamUserResource{}
)

// NewTeamUserResource creates a new team user resource.
func NewTeamUserResource() resource.Resource {
	return &TeamUserResource{}
}

// TeamUserResource manages one user's membership on a Pushover team.
type TeamUserResource struct {
	client *pushover.Client
}

// TeamUserResourceModel describes the resource data model.
type TeamUserResourceModel struct {
	Token    types.String `tfsdk:"token"`
	Email    types.String `tfsdk:"email"`
	Name     types.String `tfsdk:"name"`
	Password types.String `tfsdk:"password"`
	Instant  types.Bool   `tfsdk:"instant"`
	Admin    types.Bool   `tfsdk:"admin"`
	Group    types.String `tfsdk:"group"`
	UserID   types.String `tfsdk:"user_id"`
	ID       types.String `tfsdk:"id"`
}

func (r *TeamUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_user"
}

func (r *TeamUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Adds a user to a [Pushover team](https://pushover.net/api/teams). " +
			"`token` is that team's API token (sensitive). It is not the provider application API token.\n\n" +
			"Create calls the add-user endpoint. Delete removes the member, and falls back to revoking a " +
			"pending invitation when Pushover reports that the user is not on the team. Read lists the team " +
			"and drops this resource from state when the user id (or email, before an id is known) is absent, " +
			"so a member removed outside Terraform is recreated on the next apply.\n\n" +
			"The Teams API has no update call. Changing any argument replaces the membership: Terraform removes " +
			"the user and adds them again. At least one administrator must remain on the team.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Membership id, `team_token/user_id` (or `team_token/email` until the team listing returns a user id). Sensitive because it contains the team token.",
				Computed:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Team API token from the team's settings page. Pushover does not accept an application API token for team calls.",
				Required:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Email address of the team member. Pushover uses this address to add and remove the user.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(emailPattern, "must be an email address"),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Full name stored for this team member. Pushover has no update call, so a change replaces the membership.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Initial password for the new user. Stored in Terraform state. When omitted, Pushover generates a password and emails it. A change replaces the membership.",
				Optional:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"instant": schema.BoolAttribute{
				MarkdownDescription: "When true, the welcome email includes an Instant Login link (valid for 7 days). A change replaces the membership. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"admin": schema.BoolAttribute{
				MarkdownDescription: "When true, the user is a team administrator. Refresh copies the `administrator` flag from the team listing. A change replaces the membership. At least one administrator must remain on the team. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"group": schema.StringAttribute{
				MarkdownDescription: "Delivery group name to place the user in. Pushover creates the group if it does not exist. Omit to use the team's default delivery group. This is the group name, not a group key. A change replaces the membership.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"user_id": schema.StringAttribute{
				MarkdownDescription: "Pushover user id from the team listing. Import uses this id, not the email address.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *TeamUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pushover.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *pushover.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

func (r *TeamUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TeamUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := strings.TrimSpace(stringOrEmpty(data.Token))
	email := strings.TrimSpace(stringOrEmpty(data.Email))
	if token == "" || email == "" {
		resp.Diagnostics.AddError("Failed to add user to team", "Team token and email are required.")
		return
	}
	data.Token = types.StringValue(token)
	data.Email = types.StringValue(email)

	_, err := r.client.AddTeamUser(ctx, &pushover.AddTeamUserRequest{
		Token:    token,
		Email:    email,
		Name:     strings.TrimSpace(stringOrEmpty(data.Name)),
		Password: stringOrEmpty(data.Password),
		Instant:  boolOrFalse(data.Instant),
		Admin:    boolOrFalse(data.Admin),
		Group:    strings.TrimSpace(stringOrEmpty(data.Group)),
	})
	if err != nil && !isAlreadyOnTeamError(err) {
		resp.Diagnostics.AddError("Failed to add user to team", err.Error())
		return
	}

	// The add call does not return the user id. The team listing does.
	teamResp, err := r.client.GetTeam(ctx, token)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read team after adding user", err.Error())
		return
	}

	member, found := findTeamUser(teamResp.Users, "", email)
	if found && strings.TrimSpace(member.ID) != "" {
		// Keep the configured name, admin, and email. Refresh reconciles those.
		// Overwriting them here would replace the membership on the next plan
		// when the listing is a step behind the add call.
		data.UserID = types.StringValue(strings.TrimSpace(member.ID))
		data.ID = types.StringValue(teamUserResourceID(token, data.UserID.ValueString()))
	} else {
		// Added, but not in the listing yet (for example a pending invitation).
		// Keep state so the next refresh can adopt the user id or drop the membership.
		data.UserID = types.StringNull()
		data.ID = types.StringValue(teamUserResourceID(token, email))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TeamUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := strings.TrimSpace(stringOrEmpty(data.Token))
	if token == "" {
		resp.Diagnostics.AddError("Failed to read team", "Team token is empty.")
		return
	}

	teamResp, err := r.client.GetTeam(ctx, token)
	if err != nil {
		// An invalid token is an auth failure, not proof the member was removed.
		resp.Diagnostics.AddError("Failed to read team", err.Error())
		return
	}

	member, found := findTeamUser(teamResp.Users, stringOrEmpty(data.UserID), stringOrEmpty(data.Email))
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	applyTeamUserToModel(&data, member)
	if id := strings.TrimSpace(member.ID); id != "" {
		data.ID = types.StringValue(teamUserResourceID(token, id))
	} else if data.ID.IsNull() || data.ID.ValueString() == "" {
		data.ID = types.StringValue(teamUserResourceID(token, member.Email))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every configurable argument forces replacement. Update only copies state
	// forward if the framework invokes it.
	var data TeamUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TeamUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token := strings.TrimSpace(stringOrEmpty(data.Token))
	email := strings.TrimSpace(stringOrEmpty(data.Email))
	if token == "" || email == "" {
		resp.Diagnostics.AddError("Failed to remove user from team", "Team token and email are required to remove a team member.")
		return
	}

	if _, err := r.client.RemoveTeamUser(ctx, token, email); err != nil {
		if !isTeamMembershipNotFoundError(err) {
			resp.Diagnostics.AddError("Failed to remove user from team", err.Error())
			return
		}
		// add_user can leave a pending invitation that remove_user does not clear.
		if _, revokeErr := r.client.RevokeTeamInvitation(ctx, token, email); revokeErr != nil && !isTeamMembershipNotFoundError(revokeErr) {
			resp.Diagnostics.AddError("Failed to revoke team invitation", revokeErr.Error())
			return
		}
	}
}

// ImportState supports team_token/user_id. team_token/email is also accepted so a
// membership that does not yet have a listing id can be imported.
func (r *TeamUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	token, userID, email, err := parseTeamUserImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("token"), token)...)
	if userID != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_id"), userID)...)
	}
	if email != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("email"), email)...)
	}
	// Defaults. Read replaces admin from the team listing.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("admin"), false)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instant"), false)...)
}

// teamUserResourceID builds the canonical resource id. The second segment is the
// Pushover user id once the team listing has returned one, and the email until then.
func teamUserResourceID(token, userIDOrEmail string) string {
	return token + "/" + userIDOrEmail
}

// parseTeamUserImportID parses team_token/user_id or team_token/email.
func parseTeamUserImportID(id string) (token, userID, email string, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", "", fmt.Errorf("import ID must be team_token/user_id")
	}
	token, rest, ok := strings.Cut(id, "/")
	token = strings.TrimSpace(token)
	rest = strings.TrimSpace(rest)
	if !ok || token == "" || rest == "" || strings.Contains(rest, "/") {
		return "", "", "", fmt.Errorf("import ID must be team_token/user_id or team_token/email, got %q", id)
	}
	if strings.Contains(rest, "@") {
		return token, "", rest, nil
	}
	return token, rest, "", nil
}

// findTeamUser locates a member. A known user id is matched exactly and does not
// fall back to email, so a reused address cannot bind this resource to a new person.
// Email match is case-insensitive and is used before the listing has produced an id.
func findTeamUser(users []pushover.TeamUser, userID, email string) (pushover.TeamUser, bool) {
	userID = strings.TrimSpace(userID)
	email = strings.TrimSpace(email)
	if userID != "" {
		for _, user := range users {
			if user.ID == userID {
				return user, true
			}
		}
		return pushover.TeamUser{}, false
	}
	if email == "" {
		return pushover.TeamUser{}, false
	}
	for _, user := range users {
		if emailsEqual(user.Email, email) {
			return user, true
		}
	}
	return pushover.TeamUser{}, false
}

// applyTeamUserToModel copies listing fields into state.
// Name is updated only when the configuration already manages it, so an unset
// name does not adopt the remote value and plan a replacement on every refresh.
// Password, instant, and group are create-time inputs and are left as stored.
func applyTeamUserToModel(data *TeamUserResourceModel, member pushover.TeamUser) {
	if id := strings.TrimSpace(member.ID); id != "" {
		data.UserID = types.StringValue(id)
	}
	remoteEmail := strings.TrimSpace(member.Email)
	if remoteEmail != "" {
		stateEmail := strings.TrimSpace(stringOrEmpty(data.Email))
		if stateEmail == "" || !emailsEqual(stateEmail, remoteEmail) {
			data.Email = types.StringValue(remoteEmail)
		}
	}
	data.Admin = types.BoolValue(member.Administrator)
	if !data.Name.IsNull() && !data.Name.IsUnknown() {
		if strings.TrimSpace(member.Name) == "" {
			data.Name = types.StringNull()
		} else {
			data.Name = types.StringValue(member.Name)
		}
	}
}

func emailsEqual(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func boolOrFalse(v types.Bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return false
	}
	return v.ValueBool()
}

// isAlreadyOnTeamError reports whether add_user failed because the user is already a member.
func isAlreadyOnTeamError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "already") {
		return false
	}
	return strings.Contains(msg, "team") ||
		strings.Contains(msg, "member") ||
		strings.Contains(msg, "exist")
}

// isTeamMembershipNotFoundError reports whether remove or revoke failed because
// the user or invitation is already absent. Invalid-token errors must not match.
func isTeamMembershipNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"not a member",
		"not on the team",
		"not on team",
		"not in the team",
		"not in team",
		"user not found",
		"no such user",
		"invitation not found",
		"no invitation",
		"no pending invitation",
		"not been invited",
		"has not been invited",
		"hasn't been invited",
	}
	for _, needle := range needles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
