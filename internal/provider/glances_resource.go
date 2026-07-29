// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GlancesResource{}

// NewGlancesResource creates a new glances resource.
func NewGlancesResource() resource.Resource {
	return &GlancesResource{}
}

// GlancesResource updates a Pushover glance widget (watch complication / lock screen).
type GlancesResource struct {
	client *pushover.Client
}

// GlancesResourceModel describes the resource data model.
type GlancesResourceModel struct {
	// Required
	UserKey types.String `tfsdk:"user_key"`

	// Optional targeting / auth
	APIToken types.String `tfsdk:"api_token"`
	Device   types.String `tfsdk:"device"`

	// Glance data fields (at least one required)
	Title   types.String `tfsdk:"title"`
	Text    types.String `tfsdk:"text"`
	Subtext types.String `tfsdk:"subtext"`
	Count   types.Int64  `tfsdk:"count"`
	Percent types.Int64  `tfsdk:"percent"`

	// Computed
	ID        types.String `tfsdk:"id"`
	RequestID types.String `tfsdk:"request_id"`
}

func (r *GlancesResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_glances"
}

func (r *GlancesResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Updates a [Pushover Glances](https://pushover.net/api/glances) widget " +
			"(for example an Apple Watch complication). Glance data is low-priority state pushed to a " +
			"constantly-updated screen; it does **not** generate a notification alert or sound.\n\n" +
			"The Glances API stores the current value of each field and only overwrites fields you send. " +
			"Unset attributes are left unchanged on create; when an attribute is removed from configuration " +
			"on update (or the resource is destroyed), that field is cleared on the widget.\n\n" +
			"At least one of `title`, `text`, `subtext`, `count`, or `percent` must be set. " +
			"Throttle updates to Apple Watch widgets (Pushover recommends ≥ 20 minutes between calls).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier for this glance target (`user_key` or `user_key/device`).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_key": schema.StringAttribute{
				MarkdownDescription: "The Pushover user key whose widget(s) should receive the glance data.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Override the provider-level Pushover application API token for this resource.",
				Optional:            true,
				Sensitive:           true,
			},
			"device": schema.StringAttribute{
				MarkdownDescription: "Restrict the update to the widget on this device name. " +
					"Omit to update all of the user's registered widgets.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "Description of the data being shown (up to 100 characters), e.g. `\"Widgets Sold\"`.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"text": schema.StringAttribute{
				MarkdownDescription: "Main line of data shown on most screens (up to 100 characters).",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"subtext": schema.StringAttribute{
				MarkdownDescription: "Secondary line of data (up to 100 characters).",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"count": schema.Int64Attribute{
				MarkdownDescription: "Integer count shown on smaller screens. May be negative.",
				Optional:            true,
			},
			"percent": schema.Int64Attribute{
				MarkdownDescription: "Progress value from 0 through 100 (inclusive), shown as a bar/circle on some screens.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.Between(0, 100),
				},
			},
			"request_id": schema.StringAttribute{
				MarkdownDescription: "The unique request ID returned by the most recent Pushover Glances API call.",
				Computed:            true,
			},
		},
	}
}

func (r *GlancesResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GlancesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data GlancesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !hasGlanceDataField(data) {
		resp.Diagnostics.AddError(
			"Missing Glance Data",
			"At least one of title, text, subtext, count, or percent must be set.",
		)
		return
	}

	apiResp, err := r.client.UpdateGlance(ctx, buildGlanceRequest(data, nil))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update Pushover glance", err.Error())
		return
	}

	data.ID = types.StringValue(glanceID(data.UserKey.ValueString(), data.Device))
	data.RequestID = types.StringValue(apiResp.Request)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read does nothing: the Glances API has no endpoint to retrieve current widget state.
func (r *GlancesResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *GlancesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan GlancesResourceModel
	var state GlancesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !hasGlanceDataField(plan) {
		resp.Diagnostics.AddError(
			"Missing Glance Data",
			"At least one of title, text, subtext, count, or percent must be set.",
		)
		return
	}

	apiResp, err := r.client.UpdateGlance(ctx, buildGlanceRequest(plan, &state))
	if err != nil {
		resp.Diagnostics.AddError("Failed to update Pushover glance", err.Error())
		return
	}

	plan.ID = state.ID
	if plan.ID.IsNull() || plan.ID.IsUnknown() {
		plan.ID = types.StringValue(glanceID(plan.UserKey.ValueString(), plan.Device))
	}
	plan.RequestID = types.StringValue(apiResp.Request)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *GlancesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GlancesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Clear any fields that were previously set so the widget does not keep stale data.
	clearReq := &pushover.GlanceRequest{
		User:       state.UserKey.ValueString(),
		DataFields: map[string]string{},
	}
	if !state.APIToken.IsNull() && !state.APIToken.IsUnknown() {
		clearReq.Token = state.APIToken.ValueString()
	}
	if !state.Device.IsNull() && !state.Device.IsUnknown() {
		clearReq.Device = state.Device.ValueString()
	}
	if !state.Title.IsNull() {
		clearReq.DataFields["title"] = ""
	}
	if !state.Text.IsNull() {
		clearReq.DataFields["text"] = ""
	}
	if !state.Subtext.IsNull() {
		clearReq.DataFields["subtext"] = ""
	}
	if !state.Count.IsNull() {
		clearReq.DataFields["count"] = ""
	}
	if !state.Percent.IsNull() {
		clearReq.DataFields["percent"] = ""
	}

	if len(clearReq.DataFields) == 0 {
		return
	}

	if _, err := r.client.UpdateGlance(ctx, clearReq); err != nil {
		resp.Diagnostics.AddError("Failed to clear Pushover glance", err.Error())
		return
	}
}

func hasGlanceDataField(data GlancesResourceModel) bool {
	return !data.Title.IsNull() ||
		!data.Text.IsNull() ||
		!data.Subtext.IsNull() ||
		!data.Count.IsNull() ||
		!data.Percent.IsNull()
}

// buildGlanceRequest builds the API request from plan values.
// When prior is non-nil (update), fields that were set in prior but are now
// null are cleared with an empty string per the Glances API.
func buildGlanceRequest(plan GlancesResourceModel, prior *GlancesResourceModel) *pushover.GlanceRequest {
	req := &pushover.GlanceRequest{
		User:       plan.UserKey.ValueString(),
		DataFields: map[string]string{},
	}
	if !plan.APIToken.IsNull() && !plan.APIToken.IsUnknown() {
		req.Token = plan.APIToken.ValueString()
	}
	if !plan.Device.IsNull() && !plan.Device.IsUnknown() {
		req.Device = plan.Device.ValueString()
	}

	setOrClearString := func(name string, planVal types.String, priorSet bool) {
		if !planVal.IsNull() && !planVal.IsUnknown() {
			req.DataFields[name] = planVal.ValueString()
			return
		}
		if prior != nil && priorSet {
			req.DataFields[name] = ""
		}
	}
	setOrClearInt := func(name string, planVal types.Int64, priorSet bool) {
		if !planVal.IsNull() && !planVal.IsUnknown() {
			req.DataFields[name] = strconv.FormatInt(planVal.ValueInt64(), 10)
			return
		}
		if prior != nil && priorSet {
			req.DataFields[name] = ""
		}
	}

	priorTitle := prior != nil && !prior.Title.IsNull()
	priorText := prior != nil && !prior.Text.IsNull()
	priorSubtext := prior != nil && !prior.Subtext.IsNull()
	priorCount := prior != nil && !prior.Count.IsNull()
	priorPercent := prior != nil && !prior.Percent.IsNull()

	setOrClearString("title", plan.Title, priorTitle)
	setOrClearString("text", plan.Text, priorText)
	setOrClearString("subtext", plan.Subtext, priorSubtext)
	setOrClearInt("count", plan.Count, priorCount)
	setOrClearInt("percent", plan.Percent, priorPercent)

	return req
}

func glanceID(userKey string, device types.String) string {
	if !device.IsNull() && !device.IsUnknown() && device.ValueString() != "" {
		return userKey + "/" + device.ValueString()
	}
	return userKey
}
