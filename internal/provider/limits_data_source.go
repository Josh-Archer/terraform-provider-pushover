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
var _ datasource.DataSource = &LimitsDataSource{}

// NewLimitsDataSource creates a new limits data source.
func NewLimitsDataSource() datasource.DataSource {
	return &LimitsDataSource{}
}

// LimitsDataSource retrieves message quota limits for an application.
type LimitsDataSource struct {
	client *pushover.Client
}

// LimitsDataSourceModel describes the data source data model.
type LimitsDataSourceModel struct {
	APIToken  types.String `tfsdk:"api_token"`
	ID        types.String `tfsdk:"id"`
	Limit     types.Int64  `tfsdk:"limit"`
	Remaining types.Int64  `tfsdk:"remaining"`
	Reset     types.Int64  `tfsdk:"reset"`
}

func (d *LimitsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_limits"
}

func (d *LimitsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the current monthly message quota, remaining messages, and quota reset time for the Pushover application.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Resource identifier.",
				Computed:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Override the provider-level API token for querying limits.",
				Optional:            true,
				Sensitive:           true,
			},
			"limit": schema.Int64Attribute{
				MarkdownDescription: "The total monthly message quota for this application.",
				Computed:            true,
			},
			"remaining": schema.Int64Attribute{
				MarkdownDescription: "The number of messages remaining in the current monthly period.",
				Computed:            true,
			},
			"reset": schema.Int64Attribute{
				MarkdownDescription: "Unix timestamp indicating when the message quota will reset.",
				Computed:            true,
			},
		},
	}
}

func (d *LimitsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LimitsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LimitsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var token []string
	if !data.APIToken.IsNull() && data.APIToken.ValueString() != "" {
		token = append(token, data.APIToken.ValueString())
	}

	limits, err := d.client.GetLimits(ctx, token...)
	if err != nil {
		resp.Diagnostics.AddError("Failed to fetch Pushover application limits", err.Error())
		return
	}

	data.ID = types.StringValue("limits")
	data.Limit = types.Int64Value(int64(limits.Limit))
	data.Remaining = types.Int64Value(int64(limits.Remaining))
	data.Reset = types.Int64Value(limits.Reset)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
