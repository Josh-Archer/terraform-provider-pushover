// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ datasource.DataSource = &SoundsDataSource{}

// NewSoundsDataSource creates a new sounds data source.
func NewSoundsDataSource() datasource.DataSource {
	return &SoundsDataSource{}
}

// SoundsDataSource defines the data source implementation.
type SoundsDataSource struct {
	client *pushover.Client
}

// SoundsDataSourceModel describes the data source data model.
type SoundsDataSourceModel struct {
	Sounds types.Map    `tfsdk:"sounds"`
	Keys   types.List   `tfsdk:"keys"`
	ID     types.String `tfsdk:"id"`
}

func (d *SoundsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sounds"
}

func (d *SoundsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieves the list of available Pushover notification sounds. " +
			"Use the sound keys with the `pushover_message` resource's `sound` attribute.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Placeholder identifier.",
				Computed:            true,
			},
			"sounds": schema.MapAttribute{
				MarkdownDescription: "A map of sound key to human-readable sound name.",
				Computed:            true,
				ElementType:         types.StringType,
			},
			"keys": schema.ListAttribute{
				MarkdownDescription: "A list of available sound keys that can be used in `pushover_message`.",
				Computed:            true,
				ElementType:         types.StringType,
			},
		},
	}
}

func (d *SoundsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *SoundsDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	sounds, err := d.client.GetSounds(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to fetch Pushover sounds", err.Error())
		return
	}

	soundsMap := make(map[string]attr.Value, len(sounds))
	keysList := make([]attr.Value, 0, len(sounds))
	for _, s := range sounds {
		soundsMap[s.Key] = types.StringValue(s.Name)
		keysList = append(keysList, types.StringValue(s.Key))
	}
	sort.Slice(keysList, func(i, j int) bool {
		return keysList[i].(types.String).ValueString() < keysList[j].(types.String).ValueString()
	})

	soundsTF, diags := types.MapValue(types.StringType, soundsMap)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	keysTF, diags := types.ListValue(types.StringType, keysList)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	state := SoundsDataSourceModel{
		Sounds: soundsTF,
		Keys:   keysTF,
		ID:     types.StringValue("sounds"),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
