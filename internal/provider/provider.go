// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"os"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure PushoverProvider satisfies various provider interfaces.
var _ provider.Provider = &PushoverProvider{}
var _ provider.ProviderWithFunctions = &PushoverProvider{}

// PushoverProvider defines the provider implementation.
type PushoverProvider struct {
	version string
}

// PushoverProviderModel describes the provider data model.
type PushoverProviderModel struct {
	APIToken types.String `tfsdk:"api_token"`
}

// New creates a new instance of the Pushover provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &PushoverProvider{
			version: version,
		}
	}
}

func (p *PushoverProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "pushover"
	resp.Version = p.version
}

func (p *PushoverProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The Pushover provider allows you to send push notifications via [Pushover](https://pushover.net). " +
			"Configure the provider with your application API token to get started.",
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				MarkdownDescription: "The Pushover application API token. " +
					"Can also be set via the `PUSHOVER_API_TOKEN` environment variable.",
				Optional:  true,
				Sensitive: true,
			},
		},
	}
}

func (p *PushoverProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data PushoverProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiToken := os.Getenv("PUSHOVER_API_TOKEN")
	if !data.APIToken.IsNull() && !data.APIToken.IsUnknown() {
		apiToken = data.APIToken.ValueString()
	}

	if apiToken == "" {
		resp.Diagnostics.AddError(
			"Missing API Token",
			"The provider requires a Pushover application API token. "+
				"Set the api_token attribute or the PUSHOVER_API_TOKEN environment variable.",
		)
		return
	}

	// PUSHOVER_API_BASE_URL is an undocumented test seam for pointing the client
	// at a mock HTTP server (acceptance / plan-churn unit tests).
	if baseURL := os.Getenv("PUSHOVER_API_BASE_URL"); baseURL != "" {
		client := pushover.NewClientWithBase(apiToken, baseURL, &http.Client{})
		resp.DataSourceData = client
		resp.ResourceData = client
		return
	}

	client := pushover.NewClient(apiToken)
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *PushoverProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewMessageResource,
		NewGroupUserResource,
		NewReceiptResource,
	}
}

func (p *PushoverProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewSoundsDataSource,
		NewValidateUserDataSource,
	}
}

func (p *PushoverProvider) Functions(_ context.Context) []func() function.Function {
	return []func() function.Function{}
}
