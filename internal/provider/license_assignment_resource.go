// Copyright (c) Josh Archer
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/Josh-Archer/terraform-provider-pushover/internal/pushover"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &LicenseAssignmentResource{}

// NewLicenseAssignmentResource creates a resource that assigns a Pushover license credit.
func NewLicenseAssignmentResource() resource.Resource {
	return &LicenseAssignmentResource{}
}

// LicenseAssignmentResource assigns one pre-paid Pushover license credit to a user or email.
type LicenseAssignmentResource struct {
	client *pushover.Client
}

// LicenseAssignmentResourceModel describes the resource data model.
type LicenseAssignmentResourceModel struct {
	UserKey   types.String `tfsdk:"user_key"`
	Email     types.String `tfsdk:"email"`
	OS        types.String `tfsdk:"os"`
	APIToken  types.String `tfsdk:"api_token"`
	ID        types.String `tfsdk:"id"`
	Credits   types.Int64  `tfsdk:"credits"`
	RequestID types.String `tfsdk:"request_id"`
}

func (r *LicenseAssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_license_assignment"
}

func (r *LicenseAssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Permanently assigns one pre-paid [Pushover license credit](https://pushover.net/api/licensing) " +
			"to a Pushover user account (`user_key`) or email address (`email`) via `POST /1/licenses/assign.json`.\n\n" +
			"**Cost & billing warning:** Creating or replacing this resource permanently consumes **one pre-paid license credit** " +
			"from your Pushover application's credit balance. Purchased license credits are non-refundable, and once a license " +
			"is assigned to a user account or pending email address, it cannot be revoked or reassigned to another user.\n\n" +
			"**Plan & refresh behavior:** Assignment runs **only on create** during `terraform apply`. `Read` makes no network call " +
			"and never assigns credits during `terraform plan` or `terraform refresh`.\n\n" +
			"**Destroy semantics:** Because Pushover has no API to revoke or refund an assigned license, destroying this resource " +
			"is a no-op on the Pushover API that removes the resource from Terraform state only and emits a warning. " +
			"Changing `user_key`, `email`, `os`, or `api_token` replaces the resource and consumes an additional license credit.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Unique identifier for this license assignment (`user:<user_key>[/<os>]` or `email:<email>[/<os>]`).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_key": schema.StringAttribute{
				MarkdownDescription: "Pushover user key of the recipient. Exactly one of `user_key` or `email` must be set. Forces replacement (which consumes another credit).",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("user_key"),
						path.MatchRoot("email"),
					),
				},
			},
			"email": schema.StringAttribute{
				MarkdownDescription: "Email address of the recipient. If no Pushover account exists for this email yet, the license credit is immediately deducted and held as pending until the user registers. Exactly one of `user_key` or `email` must be set. Forces replacement (which consumes another credit).",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(emailPattern, "must be an email address"),
					stringvalidator.ExactlyOneOf(
						path.MatchRoot("user_key"),
						path.MatchRoot("email"),
					),
				},
			},
			"os": schema.StringAttribute{
				MarkdownDescription: "Optional platform restriction: `Android`, `iOS`, or `Desktop`. When omitted, the license applies to the first unlicensed platform on which the user registers a device. Forces replacement (which consumes another credit).",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf("Android", "iOS", "Desktop"),
				},
			},
			"api_token": schema.StringAttribute{
				MarkdownDescription: "Override the provider-level Pushover application API token. The token must belong to the application that owns the pre-paid license credits. Forces replacement (which consumes another credit).",
				Optional:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"credits": schema.Int64Attribute{
				MarkdownDescription: "Number of unassigned license credits remaining on the application immediately after this assignment.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"request_id": schema.StringAttribute{
				MarkdownDescription: "Pushover request ID from the license assignment call.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *LicenseAssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *LicenseAssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data LicenseAssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	userKey := strings.TrimSpace(stringOrEmpty(data.UserKey))
	email := strings.TrimSpace(stringOrEmpty(data.Email))
	if (userKey == "") == (email == "") {
		resp.Diagnostics.AddError(
			"Invalid License Recipient",
			"Exactly one of user_key or email must be set.",
		)
		return
	}

	osVal := strings.TrimSpace(stringOrEmpty(data.OS))
	if osVal != "" {
		switch osVal {
		case "Android", "iOS", "Desktop":
		default:
			resp.Diagnostics.AddError(
				"Invalid License OS",
				fmt.Sprintf("os must be one of Android, iOS, or Desktop; got %q.", osVal),
			)
			return
		}
	}

	apiResp, err := r.client.AssignLicense(ctx, &pushover.LicenseAssignRequest{
		Token: stringOrEmpty(data.APIToken),
		User:  userKey,
		Email: email,
		OS:    osVal,
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to assign Pushover license", err.Error())
		return
	}

	data.ID = types.StringValue(licenseAssignmentID(userKey, email, osVal))
	data.Credits = types.Int64Value(int64(apiResp.Credits))
	data.RequestID = types.StringValue(apiResp.Request)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read is a no-op and makes no network call.
// The Pushover Licensing API has no endpoint to inspect an existing assignment,
// and refresh/plan must never call the assignment endpoint or trigger re-assignment.
func (r *LicenseAssignmentResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *LicenseAssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Every configurable attribute forces replacement. Update only copies state
	// forward if the framework invokes it.
	var data LicenseAssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *LicenseAssignmentResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Pushover license credits cannot be revoked or refunded once assigned.
	// Destroy removes Terraform state only and warns the operator.
	resp.Diagnostics.AddWarning(
		"Pushover license assignment cannot be revoked or refunded",
		"Pushover license credits are permanently assigned and non-refundable. "+
			"Destroying this resource removes it from Terraform state only; the assigned license remains active (or pending) in Pushover and the credit is not returned.",
	)
}

func licenseAssignmentID(userKey, email, osVal string) string {
	var id string
	if userKey != "" {
		id = "user:" + userKey
	} else {
		id = "email:" + email
	}
	if osVal != "" {
		id += "/" + osVal
	}
	return id
}
