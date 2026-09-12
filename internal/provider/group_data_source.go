// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &GroupDataSource{}

// NewGroupDataSource creates a new group data source.
func NewGroupDataSource() datasource.DataSource {
	return &GroupDataSource{}
}

// GroupDataSource retrieves details about a Pushover delivery group.
type GroupDataSource struct {
	client *pushover.Client
}

// GroupMemberModel describes a group member in the data source.
type GroupMemberModel struct {
	UserKey  types.String `tfsdk:"user_key"`
	Device   types.String `tfsdk:"device"`
	Memo     types.String `tfsdk:"memo"`
	Disabled types.Bool   `tfsdk:"disabled"`
}

// GroupDataSourceModel describes the data source data model.
type GroupDataSourceModel struct {
	GroupKey types.String       `tfsdk:"group_key"`
	APIToken types.String       `tfsdk:"api_token"`
	ID       types.String       `tfsdk:"id"`
	Name     types.String       `tfsdk:"name"`
	Users    []GroupMemberModel `tfsdk:"users"`
}

func (d *GroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *GroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves details about a Pushover delivery group, including its name and member list.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The group key (used as resource identifier).",
				Computed:            true,
			},
			"group_key": schema.StringAttribute{
				MarkdownDescription: "The Pushover delivery group key to look up.",
				Required:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Override the provider-level API token for this query.",
				Optional:            true,
				Sensitive:           true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the delivery group.",
				Computed:            true,
			},
			"users": schema.ListNestedAttribute{
				MarkdownDescription: "The list of users in this delivery group.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"user_key": schema.StringAttribute{
							MarkdownDescription: "The Pushover user key.",
							Computed:            true,
						},
						"device": schema.StringAttribute{
							MarkdownDescription: "The device name to restrict notifications to, if configured.",
							Computed:            true,
						},
						"memo": schema.StringAttribute{
							MarkdownDescription: "A memo or description for this group member.",
							Computed:            true,
						},
						"disabled": schema.BoolAttribute{
							MarkdownDescription: "Whether delivery to this member is disabled.",
							Computed:            true,
						},
					},
				},
			},
		},
	}
}

func (d *GroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*pushover.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *pushover.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.client = client
}

func (d *GroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data GroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupKey := data.GroupKey.ValueString()
	var token []string
	if !data.APIToken.IsNull() && data.APIToken.ValueString() != "" {
		token = append(token, data.APIToken.ValueString())
	}

	result, err := d.client.GetGroup(ctx, groupKey, token...)
	if err != nil {
		resp.Diagnostics.AddError("Failed to fetch Pushover group", err.Error())
		return
	}

	data.ID = data.GroupKey
	data.Name = types.StringValue(result.Name)

	users := make([]GroupMemberModel, 0, len(result.Users))
	for _, u := range result.Users {
		member := GroupMemberModel{
			UserKey:  types.StringValue(u.User),
			Disabled: types.BoolValue(u.Disabled),
		}
		if u.Device != "" {
			member.Device = types.StringValue(u.Device)
		} else {
			member.Device = types.StringNull()
		}
		if u.Memo != "" {
			member.Memo = types.StringValue(u.Memo)
		} else {
			member.Memo = types.StringNull()
		}
		users = append(users, member)
	}
	data.Users = users

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
