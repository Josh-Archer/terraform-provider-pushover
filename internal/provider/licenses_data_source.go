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
var _ datasource.DataSource = &LicensesDataSource{}

// NewLicensesDataSource creates a new licenses data source.
func NewLicensesDataSource() datasource.DataSource {
	return &LicensesDataSource{}
}

// LicensesDataSource retrieves remaining pre-paid license credits for an application.
type LicensesDataSource struct {
	client *pushover.Client
}

// LicensesDataSourceModel describes the data source data model.
type LicensesDataSourceModel struct {
	APIToken  types.String `tfsdk:"api_token"`
	ID        types.String `tfsdk:"id"`
	Credits   types.Int64  `tfsdk:"credits"`
	RequestID types.String `tfsdk:"request_id"`
}

func (d *LicensesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_licenses"
}

func (d *LicensesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the number of remaining pre-paid [Pushover license credits](https://pushover.net/api/licensing) " +
			"available to the Pushover application (`GET /1/licenses.json`). Read-only; never assigns or consumes credits.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Static data source identifier (`licenses`).",
				Computed:            true,
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Override the provider-level application API token for querying remaining license credits.",
				Optional:            true,
				Sensitive:           true,
			},
			"credits": schema.Int64Attribute{
				MarkdownDescription: "The number of unassigned pre-paid license credits remaining on the application.",
				Computed:            true,
			},
			"request_id": schema.StringAttribute{
				MarkdownDescription: "The unique request ID returned by the Pushover Licensing API.",
				Computed:            true,
			},
		},
	}
}

func (d *LicensesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *LicensesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data LicensesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var token []string
	if !data.APIToken.IsNull() && !data.APIToken.IsUnknown() && data.APIToken.ValueString() != "" {
		token = append(token, data.APIToken.ValueString())
	}

	licenses, err := d.client.GetLicenses(ctx, token...)
	if err != nil {
		resp.Diagnostics.AddError("Failed to fetch Pushover license credits", err.Error())
		return
	}

	data.ID = types.StringValue("licenses")
	data.Credits = types.Int64Value(int64(licenses.Credits))
	data.RequestID = types.StringValue(licenses.Request)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
