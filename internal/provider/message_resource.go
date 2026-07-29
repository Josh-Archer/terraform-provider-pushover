// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &MessageResource{}

// NewMessageResource creates a new message resource.
func NewMessageResource() resource.Resource {
	return &MessageResource{}
}

// MessageResource defines the resource implementation.
type MessageResource struct {
	client *pushover.Client
}

// MessageResourceModel describes the resource data model.
type MessageResourceModel struct {
	// Required
	UserKey types.String `tfsdk:"user_key"`
	Message types.String `tfsdk:"message"`

	// Optional sending fields
	APIToken  types.String `tfsdk:"api_token"`
	Title     types.String `tfsdk:"title"`
	URL       types.String `tfsdk:"url"`
	URLTitle  types.String `tfsdk:"url_title"`
	Priority  types.Int64  `tfsdk:"priority"`
	Sound     types.String `tfsdk:"sound"`
	Device    types.String `tfsdk:"device"`
	Timestamp types.Int64  `tfsdk:"timestamp"`
	HTML      types.Bool   `tfsdk:"html"`
	Monospace types.Bool   `tfsdk:"monospace"`
	TTL       types.Int64  `tfsdk:"ttl"`

	// Attachment fields
	Attachment     types.String `tfsdk:"attachment"`
	AttachmentType types.String `tfsdk:"attachment_type"`

	// Emergency priority (priority=2) fields
	Retry    types.Int64  `tfsdk:"retry"`
	Expire   types.Int64  `tfsdk:"expire"`
	Callback types.String `tfsdk:"callback"`

	// Idempotency control (not sent to the API)
	IdempotencyKey types.String `tfsdk:"idempotency_key"`

	// Computed
	Receipt   types.String `tfsdk:"receipt"`
	RequestID types.String `tfsdk:"request_id"`
}

func (r *MessageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_message"
}

func (r *MessageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Sends a Pushover notification. The message is delivered when this resource is created. " +
			"Because a sent message cannot be updated via the API, **every configuration change forces replacement** and re-sends the notification.\n\n" +
			"To prevent accidental re-sends from unrelated plan churn, prefer a one-shot lifecycle " +
			"(`lifecycle { ignore_changes = all }`), an intentional `idempotency_key`, and/or " +
			"`lifecycle.replace_triggered_by`. See the resource documentation for patterns.",
		Attributes: map[string]schema.Attribute{
			"user_key": schema.StringAttribute{
				MarkdownDescription: "The Pushover user or group key to deliver the message to. Forces replacement.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"message": schema.StringAttribute{
				MarkdownDescription: "The message body (up to 1024 characters). Supports HTML if `html` is enabled. Forces replacement.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 1024),
				},
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Override the provider-level Pushover application API token for this message. Forces replacement.",
				Optional:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"title": schema.StringAttribute{
				MarkdownDescription: "The message title (up to 250 characters). Defaults to the application name. Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtMost(250),
				},
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "A supplementary URL to show with the message (up to 512 characters). Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtMost(512),
				},
			},
			"url_title": schema.StringAttribute{
				MarkdownDescription: "A title for the supplementary URL (up to 100 characters). Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtMost(100),
				},
			},
			"priority": schema.Int64Attribute{
				MarkdownDescription: "Message priority: `-2` (lowest), `-1` (low), `0` (normal, default), `1` (high), `2` (emergency). Forces replacement.",
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Validators: []validator.Int64{
					int64validator.Between(-2, 2),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"sound": schema.StringAttribute{
				MarkdownDescription: "The name of a Pushover sound to override the user's default. Use the `pushover_sounds` data source to list available sounds. Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"device": schema.StringAttribute{
				MarkdownDescription: "The name of a specific device to deliver the message to, rather than all of the user's devices. Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"timestamp": schema.Int64Attribute{
				MarkdownDescription: "A Unix timestamp to display instead of the time the message was received. Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"html": schema.BoolAttribute{
				MarkdownDescription: "Set to `true` to enable HTML formatting in the message body. Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"monospace": schema.BoolAttribute{
				MarkdownDescription: "Set to `true` to display the message in a monospace font. Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
				},
			},
			"ttl": schema.Int64Attribute{
				MarkdownDescription: "Time to live in seconds. The message is deleted from Pushover servers after this period. Forces replacement.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"retry": schema.Int64Attribute{
				MarkdownDescription: "How often (in seconds) to re-send an emergency message until acknowledged. Required when `priority` is `2`. Minimum: 30. Forces replacement.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(30),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"expire": schema.Int64Attribute{
				MarkdownDescription: "How long (in seconds) to continue re-sending an emergency message. Required when `priority` is `2`. Maximum: 10800. Forces replacement.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.Between(1, 10800),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"callback": schema.StringAttribute{
				MarkdownDescription: "A URL to ping when an emergency message has been acknowledged. Forces replacement.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"idempotency_key": schema.StringAttribute{
				MarkdownDescription: "Optional opaque key that forces replacement when it changes. " +
					"Use this with `lifecycle.ignore_changes` on message content attributes so that " +
					"only intentional key updates re-send the notification (for example `release-${var.version}`). " +
					"This value is stored in state only and is never sent to the Pushover API.",
				Optional: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 256),
				},
			},
			"receipt": schema.StringAttribute{
				MarkdownDescription: "Receipt token returned for emergency (`priority = 2`) messages. Use the `pushover_receipt` resource to track acknowledgement status and cancel outstanding retries when the incident is resolved.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"request_id": schema.StringAttribute{
				MarkdownDescription: "The unique request ID returned by the Pushover API.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *MessageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *MessageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data MessageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	msgReq := &pushover.MessageRequest{
		User:    data.UserKey.ValueString(),
		Message: data.Message.ValueString(),
	}
	if !data.APIToken.IsNull() && !data.APIToken.IsUnknown() {
		msgReq.Token = data.APIToken.ValueString()
	}
	if !data.Title.IsNull() {
		msgReq.Title = data.Title.ValueString()
	}
	if !data.URL.IsNull() {
		msgReq.URL = data.URL.ValueString()
	}
	if !data.URLTitle.IsNull() {
		msgReq.URLTitle = data.URLTitle.ValueString()
	}
	if !data.Priority.IsNull() {
		msgReq.Priority = int(data.Priority.ValueInt64())
	}
	if !data.Sound.IsNull() {
		msgReq.Sound = data.Sound.ValueString()
	}
	if !data.Device.IsNull() {
		msgReq.Device = data.Device.ValueString()
	}
	if !data.Timestamp.IsNull() {
		msgReq.Timestamp = data.Timestamp.ValueInt64()
	}
	if !data.HTML.IsNull() && data.HTML.ValueBool() {
		msgReq.HTML = 1
	}
	if !data.Monospace.IsNull() && data.Monospace.ValueBool() {
		msgReq.Monospace = 1
	}
	if !data.TTL.IsNull() {
		msgReq.TTL = int(data.TTL.ValueInt64())
	}
	if msgReq.Priority == 2 {
		if data.Retry.IsNull() || data.Expire.IsNull() {
			resp.Diagnostics.AddError(
				"Missing Emergency Fields",
				"When priority is 2 (emergency), both retry and expire must be set.",
			)
			return
		}
		msgReq.Retry = int(data.Retry.ValueInt64())
		msgReq.Expire = int(data.Expire.ValueInt64())
		if !data.Callback.IsNull() {
			msgReq.Callback = data.Callback.ValueString()
		}
	}
	if !data.Attachment.IsNull() && !data.Attachment.IsUnknown() {
		msgReq.Attachment = data.Attachment.ValueString()
	}
	if !data.AttachmentType.IsNull() && !data.AttachmentType.IsUnknown() {
		msgReq.AttachmentType = data.AttachmentType.ValueString()
	}
	if msgReq.AttachmentType != "" && msgReq.Attachment == "" {
		resp.Diagnostics.AddError(
			"Invalid Attachment Configuration",
			"attachment_type requires attachment to be set.",
		)
		return
	}

	result, err := r.client.SendMessage(ctx, msgReq)
	if err != nil {
		resp.Diagnostics.AddError("Failed to send Pushover message", err.Error())
		return
	}

	data.Receipt = types.StringValue(result.Receipt)
	data.RequestID = types.StringValue(result.Request)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read does nothing since Pushover messages cannot be retrieved after sending.
func (r *MessageResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {}

// Update is not used; all changes require replacement.
func (r *MessageResource) Update(_ context.Context, _ resource.UpdateRequest, _ *resource.UpdateResponse) {
}

// Delete does nothing since Pushover messages cannot be deleted.
func (r *MessageResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}
