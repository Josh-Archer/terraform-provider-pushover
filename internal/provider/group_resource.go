// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &GroupResource{}
	_ resource.ResourceWithImportState = &GroupResource{}
)

// NewGroupResource creates a resource for managing a Pushover delivery group's name.
func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

// GroupResource manages the name of an existing Pushover delivery group.
type GroupResource struct {
	client *pushover.Client
}

// GroupResourceModel describes the group resource state.
type GroupResourceModel struct {
	ID       types.String `tfsdk:"id"`
	GroupKey types.String `tfsdk:"group_key"`
	Name     types.String `tfsdk:"name"`
}

func (r *GroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (r *GroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the name of an existing Pushover delivery group. Pushover groups must first be created in the Pushover dashboard or through the Pushover API. Destroying this resource stops managing the group; it does not delete the group or its members.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The Pushover delivery group key.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_key": schema.StringAttribute{
				MarkdownDescription: "The key of the existing Pushover delivery group to manage.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The delivery group's name. Changes rename the group in Pushover.",
				Required:            true,
			},
		},
	}
}

func (r *GroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	if _, err := r.client.RenameGroup(ctx, groupKey, data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to rename Pushover group", err.Error())
		return
	}

	data.ID = types.StringValue(groupKey)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	group, err := r.client.GetGroup(ctx, groupKey)
	if err != nil {
		if isGroupNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read Pushover group", err.Error())
		return
	}

	data.ID = types.StringValue(groupKey)
	data.Name = types.StringValue(group.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	if _, err := r.client.RenameGroup(ctx, groupKey, data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to rename Pushover group", err.Error())
		return
	}

	data.ID = types.StringValue(groupKey)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GroupResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Pushover does not expose a group deletion endpoint. Dropping Terraform state
	// leaves the group and its members untouched.
}

func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_key"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
