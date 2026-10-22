// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// deviceNamePattern matches Pushover device names: 1–25 characters from [A-Za-z0-9_-].
var deviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,25}$`)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &SubscriptionSubscriberResource{}

// NewSubscriptionSubscriberResource creates a resource that migrates a user onto a subscription.
func NewSubscriptionSubscriberResource() resource.Resource {
	return &SubscriptionSubscriberResource{}
}

// SubscriptionSubscriberResource migrates one Pushover user onto an application subscription.
type SubscriptionSubscriberResource struct {
	client *pushover.Client
}

// SubscriptionSubscriberResourceModel describes the resource data model.
type SubscriptionSubscriberResourceModel struct {
	Subscription      types.String `tfsdk:"subscription"`
	UserKey           types.String `tfsdk:"user_key"`
	DeviceName        types.String `tfsdk:"device_name"`
	Sound             types.String `tfsdk:"sound"`
	APIToken          types.String `tfsdk:"api_token"`
	ID                types.String `tfsdk:"id"`
	SubscribedUserKey types.String `tfsdk:"subscribed_user_key"`
	RequestID         types.String `tfsdk:"request_id"`
}

func (r *SubscriptionSubscriberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_subscriber"
}

func (r *SubscriptionSubscriberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Migrates an existing Pushover user key onto an application [subscription](https://pushover.net/api/subscriptions) " +
			"and stores the returned `subscribed_user_key`. The subscription must already exist in the Pushover dashboard.\n\n" +
			"Send later notifications to `subscribed_user_key` (or to the delivery group backing the subscription). " +
			"Pushover has no API to list subscribers or to unsubscribe a user. Destroying this resource removes it from " +
			"Terraform state only; the person remains subscribed until they unsubscribe.\n\n" +
			"Changing `subscription`, `user_key`, `device_name`, `sound`, or `api_token` replaces the resource and " +
			"calls the migration API again. That does not remove the previous subscription.\n\n" +
			"On refresh, the provider validates `subscribed_user_key`. If Pushover reports that key is no longer a valid " +
			"user (for example the user unsubscribed and the key was deleted), the resource is removed from state so the " +
			"next plan migrates again. Other API errors leave state in place and fail the refresh.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier for this migration (`subscription/user_key` or `subscription/user_key/device_name`).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subscription": schema.StringAttribute{
				MarkdownDescription: "Subscription code from the Pushover dashboard (the code in `https://pushover.net/subscribe/CODE`).",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"user_key": schema.StringAttribute{
				MarkdownDescription: "Existing Pushover user key to migrate onto the subscription.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"device_name": schema.StringAttribute{
				MarkdownDescription: "Optionally limit the subscription to this device name (Pushover parameter `device_name`).",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						deviceNamePattern,
						"device name must be 1-25 characters and contain only letters, numbers, underscores, or hyphens",
					),
				},
			},
			"sound": schema.StringAttribute{
				MarkdownDescription: "Optional preferred default sound for this subscriber. Omit to keep the user's default.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Override the provider-level Pushover application API token for this migration. The token must belong to the application that owns the subscription.",
				Optional:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"subscribed_user_key": schema.StringAttribute{
				MarkdownDescription: "Subscription user key returned by Pushover. Store this in place of the original user key when sending messages to this subscriber.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"request_id": schema.StringAttribute{
				MarkdownDescription: "Pushover request ID from the migration call.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *SubscriptionSubscriberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *SubscriptionSubscriberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SubscriptionSubscriberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiResp, err := r.client.MigrateSubscriptionUser(ctx, &pushover.SubscriptionMigrateRequest{
		Token:        stringOrEmpty(data.APIToken),
		Subscription: data.Subscription.ValueString(),
		User:         data.UserKey.ValueString(),
		DeviceName:   stringOrEmpty(data.DeviceName),
		Sound:        stringOrEmpty(data.Sound),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to migrate user onto subscription", err.Error())
		return
	}

	data.ID = types.StringValue(subscriptionSubscriberID(
		data.Subscription.ValueString(),
		data.UserKey.ValueString(),
		stringOrEmpty(data.DeviceName),
	))
	data.SubscribedUserKey = types.StringValue(apiResp.SubscribedUserKey)
	data.RequestID = types.StringValue(apiResp.Request)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SubscriptionSubscriberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SubscriptionSubscriberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	subscribedKey := stringOrEmpty(data.SubscribedUserKey)
	if subscribedKey == "" {
		// Nothing to validate. Drop state so the next plan migrates again.
		resp.State.RemoveResource(ctx)
		return
	}

	_, err := r.client.ValidateUser(ctx, &pushover.ValidateRequest{
		Token: stringOrEmpty(data.APIToken),
		User:  subscribedKey,
	})
	if err != nil {
		if isSubscribedUserGoneError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to validate subscription subscriber", err.Error())
		return
	}

	// The migration API does not return device_name or sound, and validate does not
	// either. Leave configured attributes untouched when the key is still valid.
}

func (r *SubscriptionSubscriberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every configurable attribute forces replacement. Update only copies state
	// forward if the framework invokes it.
	var data SubscriptionSubscriberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *SubscriptionSubscriberResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Pushover's Subscription API has no unsubscribe endpoint. Destroy removes
	// Terraform state only; the user stays subscribed until they unsubscribe.
}

// subscriptionSubscriberID builds the canonical resource id.
func subscriptionSubscriberID(subscription, userKey, deviceName string) string {
	id := subscription + "/" + userKey
	if deviceName != "" {
		id += "/" + deviceName
	}
	return id
}

func stringOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// isSubscribedUserGoneError reports whether ValidateUser failed because the
// subscribed user key itself is no longer valid. Unrelated failures (bad
// application token, transport errors, generic "not found") must not match,
// so refresh keeps state instead of planning a new migration.
func isSubscribedUserGoneError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"user key is invalid",
		"user key is not valid",
		"invalid user",
		"not a valid user",
		"user identifier is not",
		"user not found",
		"no such user",
	}
	for _, needle := range needles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}
