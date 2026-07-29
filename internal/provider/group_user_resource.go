// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &GroupUserResource{}
	_ resource.ResourceWithImportState = &GroupUserResource{}
)

// NewGroupUserResource creates a new group user resource.
func NewGroupUserResource() resource.Resource {
	return &GroupUserResource{}
}

// GroupUserResource manages a user's membership in a Pushover delivery group.
type GroupUserResource struct {
	client *pushover.Client
}

// GroupUserResourceModel describes the resource data model.
type GroupUserResourceModel struct {
	GroupKey types.String `tfsdk:"group_key"`
	UserKey  types.String `tfsdk:"user_key"`
	Device   types.String `tfsdk:"device"`
	Memo     types.String `tfsdk:"memo"`
	Disabled types.Bool   `tfsdk:"disabled"`
	// Computed ID to ensure uniqueness in state
	ID types.String `tfsdk:"id"`
}

func (r *GroupUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_user"
}

func (r *GroupUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Adds a user to a Pushover delivery group. The group must already exist in Pushover. " +
			"The group key is typically obtained from the Pushover dashboard or from a `pushover_group` resource.\n\n" +
			"On refresh, this resource re-reads group membership from the Pushover API. If the user is no longer " +
			"a member (removed outside Terraform), the resource is removed from state so the next plan recreates it. " +
			"Enable/disable and memo changes made outside Terraform are also detected on refresh.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier for this group membership (`group_key/user_key[/device]`).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_key": schema.StringAttribute{
				MarkdownDescription: "The Pushover delivery group key.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_key": schema.StringAttribute{
				MarkdownDescription: "The Pushover user key to add to the group.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"device": schema.StringAttribute{
				MarkdownDescription: "Optionally restrict notifications to a specific device for this user.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"memo": schema.StringAttribute{
				MarkdownDescription: "An optional note about this group member (up to 200 characters).",
				Optional:            true,
			},
			"disabled": schema.BoolAttribute{
				MarkdownDescription: "Set to `true` to disable notifications to this user without removing them from the group.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *GroupUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GroupUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	userKey := data.UserKey.ValueString()
	device := data.Device.ValueString()
	memo := data.Memo.ValueString()

	_, err := r.client.AddGroupUser(ctx, groupKey, userKey, device, memo)
	if err != nil {
		resp.Diagnostics.AddError("Failed to add user to group", err.Error())
		return
	}

	data.ID = types.StringValue(groupUserResourceID(groupKey, userKey, device))

	// Apply disabled state if requested
	if !data.Disabled.IsNull() && data.Disabled.ValueBool() {
		if _, err := r.client.DisableGroupUser(ctx, groupKey, userKey, device); err != nil {
			resp.Diagnostics.AddError("Failed to disable group user", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	userKey := data.UserKey.ValueString()
	device := data.Device.ValueString()

	groupResp, err := r.client.GetGroup(ctx, groupKey)
	if err != nil {
		// Treat missing/invalid groups as the membership being gone so Terraform
		// can plan recreation after external deletion, rather than failing refresh.
		if isGroupNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read group", err.Error())
		return
	}

	member, found := findGroupMember(groupResp.Users, userKey, device)
	if !found {
		// User has been removed externally – drop from state so plan recreates.
		resp.State.RemoveResource(ctx)
		return
	}

	// Sync remote enable/disable and memo so external changes show as drift.
	applyGroupMemberToModel(&data, member)
	// Keep id stable even when imported without it.
	if data.ID.IsNull() || data.ID.ValueString() == "" {
		data.ID = types.StringValue(groupUserResourceID(groupKey, userKey, device))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data, state GroupUserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	userKey := data.UserKey.ValueString()
	device := data.Device.ValueString()

	// Handle memo update by re-adding
	if data.Memo != state.Memo {
		if _, err := r.client.AddGroupUser(ctx, groupKey, userKey, device, data.Memo.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to update group user memo", err.Error())
			return
		}
	}

	// Handle enable/disable toggle
	if data.Disabled != state.Disabled {
		if data.Disabled.ValueBool() {
			if _, err := r.client.DisableGroupUser(ctx, groupKey, userKey, device); err != nil {
				resp.Diagnostics.AddError("Failed to disable group user", err.Error())
				return
			}
		} else {
			if _, err := r.client.EnableGroupUser(ctx, groupKey, userKey, device); err != nil {
				resp.Diagnostics.AddError("Failed to enable group user", err.Error())
				return
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data GroupUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	userKey := data.UserKey.ValueString()
	device := data.Device.ValueString()

	if _, err := r.client.RemoveGroupUser(ctx, groupKey, userKey, device); err != nil {
		// Already removed outside Terraform is success for delete.
		if isGroupMembershipNotFoundError(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to remove user from group", err.Error())
		return
	}
}

// ImportState supports IDs of the form group_key/user_key or group_key/user_key/device.
func (r *GroupUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	groupKey, userKey, device, err := parseGroupUserImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_key"), groupKey)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user_key"), userKey)...)
	if device != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("device"), device)...)
	}
	// disabled is computed; Read will populate it from the API after import.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("disabled"), false)...)
}

// groupUserResourceID builds the canonical resource id.
func groupUserResourceID(groupKey, userKey, device string) string {
	id := groupKey + "/" + userKey
	if device != "" {
		id += "/" + device
	}
	return id
}

// parseGroupUserImportID parses group_key/user_key[/device].
func parseGroupUserImportID(id string) (groupKey, userKey, device string, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", "", fmt.Errorf("import ID must be group_key/user_key or group_key/user_key/device")
	}
	parts := strings.Split(id, "/")
	if len(parts) < 2 || len(parts) > 3 {
		return "", "", "", fmt.Errorf("import ID must be group_key/user_key or group_key/user_key/device, got %q", id)
	}
	groupKey = strings.TrimSpace(parts[0])
	userKey = strings.TrimSpace(parts[1])
	if groupKey == "" || userKey == "" {
		return "", "", "", fmt.Errorf("import ID must include non-empty group_key and user_key, got %q", id)
	}
	if len(parts) == 3 {
		device = strings.TrimSpace(parts[2])
		if device == "" {
			return "", "", "", fmt.Errorf("device segment in import ID must be non-empty when present, got %q", id)
		}
	}
	return groupKey, userKey, device, nil
}

// findGroupMember locates a group membership by user key and optional device.
// Device is matched exactly: empty device means membership not restricted to a device.
func findGroupMember(members []pushover.GroupMember, userKey, device string) (pushover.GroupMember, bool) {
	for _, member := range members {
		if member.User != userKey {
			continue
		}
		// Exact device match (both empty, or same device name).
		if member.Device == device {
			return member, true
		}
	}
	return pushover.GroupMember{}, false
}

// applyGroupMemberToModel writes remote membership fields into the resource model.
func applyGroupMemberToModel(data *GroupUserResourceModel, member pushover.GroupMember) {
	data.Disabled = types.BoolValue(member.Disabled)
	if member.Memo != "" {
		data.Memo = types.StringValue(member.Memo)
	} else if !data.Memo.IsNull() && data.Memo.ValueString() == "" {
		// Keep empty optional memo as empty string if already represented that way.
		data.Memo = types.StringValue("")
	} else {
		// Remote memo cleared / unset — clear optional attribute in state.
		data.Memo = types.StringNull()
	}
}

// isGroupNotFoundError reports whether a GetGroup error indicates the group is gone.
func isGroupNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "group not found") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "no such group") ||
		strings.Contains(msg, "invalid group")
}

// isGroupMembershipNotFoundError reports whether a remove/enable/disable error
// indicates the user is already absent from the group.
func isGroupMembershipNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not in group") ||
		strings.Contains(msg, "not a member") ||
		strings.Contains(msg, "user not found") ||
		strings.Contains(msg, "not found")
}
