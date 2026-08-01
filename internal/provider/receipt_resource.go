// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ReceiptResource{}

// NewReceiptResource creates a new emergency receipt resource.
func NewReceiptResource() resource.Resource {
	return &ReceiptResource{}
}

// ReceiptResource tracks an emergency (priority 2) message receipt and can cancel
// outstanding retries when the resource is destroyed (incident resolved).
type ReceiptResource struct {
	client *pushover.Client
}

// ReceiptResourceModel describes the resource data model.
type ReceiptResourceModel struct {
	// Required
	Receipt types.String `tfsdk:"receipt"`

	// Optional lifecycle control
	CancelOnDestroy types.Bool `tfsdk:"cancel_on_destroy"`

	// Computed status from the Pushover receipts API
	ID                   types.String `tfsdk:"id"`
	Acknowledged         types.Bool   `tfsdk:"acknowledged"`
	AcknowledgedAt       types.Int64  `tfsdk:"acknowledged_at"`
	AcknowledgedBy       types.String `tfsdk:"acknowledged_by"`
	AcknowledgedByDevice types.String `tfsdk:"acknowledged_by_device"`
	LastDeliveredAt      types.Int64  `tfsdk:"last_delivered_at"`
	Expired              types.Bool   `tfsdk:"expired"`
	ExpiresAt            types.Int64  `tfsdk:"expires_at"`
	CalledBack           types.Bool   `tfsdk:"called_back"`
	CalledBackAt         types.Int64  `tfsdk:"called_back_at"`
	RequestID            types.String `tfsdk:"request_id"`
}

func (r *ReceiptResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_receipt"
}

func (r *ReceiptResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Tracks an emergency (`priority = 2`) Pushover message receipt and optionally cancels " +
			"outstanding retries when the resource is destroyed (for example when an incident is resolved).\n\n" +
			"Create this resource with the `receipt` attribute from a `pushover_message` that was sent with " +
			"`priority = 2`. Terraform will refresh acknowledgement and expiry status on read. By default, " +
			"destroying the resource calls the Pushover cancel API so users stop receiving repeated emergency alerts.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Same as `receipt`; used as the Terraform resource identifier.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"receipt": schema.StringAttribute{
				MarkdownDescription: "Emergency receipt token returned by `pushover_message.receipt` when `priority = 2`.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cancel_on_destroy": schema.BoolAttribute{
				MarkdownDescription: "When `true` (default), destroying this resource cancels the outstanding emergency " +
					"notification via the Pushover receipts cancel API. Set to `false` to stop tracking without cancelling.",
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(true),
			},
			"acknowledged": schema.BoolAttribute{
				MarkdownDescription: "`true` if any recipient has acknowledged the emergency message.",
				Computed:            true,
			},
			"acknowledged_at": schema.Int64Attribute{
				MarkdownDescription: "Unix timestamp when the message was first acknowledged, or `0` if not acknowledged.",
				Computed:            true,
			},
			"acknowledged_by": schema.StringAttribute{
				MarkdownDescription: "User key of the recipient who acknowledged the message, if any.",
				Computed:            true,
			},
			"acknowledged_by_device": schema.StringAttribute{
				MarkdownDescription: "Device name that acknowledged the message, if any.",
				Computed:            true,
			},
			"last_delivered_at": schema.Int64Attribute{
				MarkdownDescription: "Unix timestamp of the most recent delivery attempt.",
				Computed:            true,
			},
			"expired": schema.BoolAttribute{
				MarkdownDescription: "`true` if the emergency notification has expired (retry window ended).",
				Computed:            true,
			},
			"expires_at": schema.Int64Attribute{
				MarkdownDescription: "Unix timestamp when the emergency notification will (or did) expire.",
				Computed:            true,
			},
			"called_back": schema.BoolAttribute{
				MarkdownDescription: "`true` if the optional callback URL was successfully invoked.",
				Computed:            true,
			},
			"called_back_at": schema.Int64Attribute{
				MarkdownDescription: "Unix timestamp when the callback URL was invoked, or `0` if not called.",
				Computed:            true,
			},
			"request_id": schema.StringAttribute{
				MarkdownDescription: "The unique request ID returned by the most recent receipts API call.",
				Computed:            true,
			},
		},
	}
}

func (r *ReceiptResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ReceiptResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ReceiptResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.refreshReceipt(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Failed to read Pushover receipt", err.Error())
		return
	}

	data.ID = types.StringValue(data.Receipt.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ReceiptResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ReceiptResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.refreshReceipt(ctx, &data); err != nil {
		// Receipt may no longer be queryable after long expiry windows.
		if isReceiptNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read Pushover receipt", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ReceiptResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ReceiptResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only cancel_on_destroy is updatable in place; refresh status while applying.
	if err := r.refreshReceipt(ctx, &data); err != nil {
		if isReceiptNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read Pushover receipt", err.Error())
		return
	}

	data.ID = types.StringValue(data.Receipt.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ReceiptResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ReceiptResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cancel := true
	if !data.CancelOnDestroy.IsNull() && !data.CancelOnDestroy.IsUnknown() {
		cancel = data.CancelOnDestroy.ValueBool()
	}

	if !cancel {
		return
	}

	// Skip cancel when already fully resolved to avoid noisy API errors.
	if data.Acknowledged.ValueBool() || data.Expired.ValueBool() {
		return
	}

	if _, err := r.client.CancelReceipt(ctx, data.Receipt.ValueString()); err != nil {
		// Treat already-cancelled / not-found as success so destroy is idempotent.
		if isReceiptAlreadyResolved(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to cancel Pushover emergency receipt", err.Error())
		return
	}
}

func (r *ReceiptResource) refreshReceipt(ctx context.Context, data *ReceiptResourceModel) error {
	result, err := r.client.GetReceipt(ctx, data.Receipt.ValueString())
	if err != nil {
		return err
	}

	data.Acknowledged = types.BoolValue(result.Acknowledged == 1)
	data.AcknowledgedAt = types.Int64Value(result.AcknowledgedAt)
	data.AcknowledgedBy = types.StringValue(result.AcknowledgedBy)
	data.AcknowledgedByDevice = types.StringValue(result.AcknowledgedByDevice)
	data.LastDeliveredAt = types.Int64Value(result.LastDeliveredAt)
	data.Expired = types.BoolValue(result.Expired == 1)
	data.ExpiresAt = types.Int64Value(result.ExpiresAt)
	data.CalledBack = types.BoolValue(result.CalledBack == 1)
	data.CalledBackAt = types.Int64Value(result.CalledBackAt)
	data.RequestID = types.StringValue(result.Request)
	return nil
}

func isReceiptNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "invalid receipt") ||
		strings.Contains(msg, "receipt not found") ||
		strings.Contains(msg, "no such receipt")
}

func isReceiptAlreadyResolved(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return isReceiptNotFound(err) ||
		strings.Contains(msg, "already") ||
		strings.Contains(msg, "expired") ||
		strings.Contains(msg, "acknowledged") ||
		strings.Contains(msg, "cancelled") ||
		strings.Contains(msg, "canceled")
}
