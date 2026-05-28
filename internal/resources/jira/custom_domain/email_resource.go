// Package custom_domain implements the atlassian_jira_custom_email managed resource.
//
// This resource manages Jira custom email addresses through the Atlassian Cloud
// REST API. It supports Create, Read, Delete, and ImportState operations.
// Email addresses are immutable; changing email_address or domain_id forces replacement.
package custom_domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the EmailResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &EmailResource{}
	_ resource.ResourceWithImportState = &EmailResource{}
)

// apiEmail represents the JSON structure returned by the Atlassian email API.
type apiEmail struct {
	ID           string `json:"id"`
	EmailAddress string `json:"emailAddress"`
	DomainID     string `json:"domainId"`
	SpaceID      string `json:"spaceId,omitempty"`
	Active       bool   `json:"active"`
}

// apiEmailCreate represents the JSON body for creating an email address.
type apiEmailCreate struct {
	EmailAddress string `json:"emailAddress"`
	DomainID     string `json:"domainId"`
	SpaceID      string `json:"spaceId,omitempty"`
	Active       bool   `json:"active"`
}

// EmailResourceModel describes the email resource data model.
type EmailResourceModel struct {
	ID           types.String `tfsdk:"id"`
	EmailAddress types.String `tfsdk:"email_address"`
	DomainID     types.String `tfsdk:"domain_id"`
	SpaceID      types.String `tfsdk:"space_id"`
	Active       types.Bool   `tfsdk:"active"`
}

// EmailResource implements the atlassian_jira_custom_email managed resource.
type EmailResource struct {
	client *atlassian.Client
}

// NewEmailResource returns a new EmailResource instance for provider registration.
func NewEmailResource() resource.Resource {
	return &EmailResource{}
}

// Metadata returns the resource type name.
func (r *EmailResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_custom_email"
}

// Schema defines the schema for the custom email resource.
func (r *EmailResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira custom email address. Email addresses are immutable; changing email_address or domain_id forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the custom email address, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email_address": schema.StringAttribute{
				Description: "The email address to register (e.g., support@example.com). Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"domain_id": schema.StringAttribute{
				Description: "The ID of the verified custom domain this email belongs to. Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the Jira space to associate with this email address. Optional.",
				Optional:    true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the email address is active. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *EmailResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create provisions a new custom email address.
func (r *EmailResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EmailResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiEmailCreate{
		EmailAddress: plan.EmailAddress.ValueString(),
		DomainID:     plan.DomainID.ValueString(),
		Active:       plan.Active.ValueBool(),
	}
	if !plan.SpaceID.IsNull() && !plan.SpaceID.IsUnknown() {
		body.SpaceID = plan.SpaceID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiEmail
	err := r.client.Post(ctx, "/rest/api/3/email", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate email address",
					fmt.Sprintf("The email address %q is already registered. Each email address can only be registered once.", plan.EmailAddress.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid email address",
					fmt.Sprintf("The email address %q is not valid. Provide a valid email address (e.g., support@example.com).", plan.EmailAddress.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to register custom email addresses. Ensure the service account has Jira admin privileges.",
				)
				return
			case http.StatusUnprocessableEntity:
				resp.Diagnostics.AddError(
					"Domain not verified",
					fmt.Sprintf("The domain with ID %q is not verified. Verify the domain before creating email addresses.", plan.DomainID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create custom email",
			fmt.Sprintf("Could not register email %q: %s", plan.EmailAddress.ValueString(), err.Error()),
		)
		return
	}

	mapEmailAPIToModel(&created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *EmailResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EmailResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var email apiEmail
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/email/%s", state.ID.ValueString()), &email)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read custom email",
			fmt.Sprintf("Could not read email with ID %q: %s. Verify the email ID is correct and the email has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	mapEmailAPIToModel(&email, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is not supported for custom email addresses. Email addresses are immutable.
func (r *EmailResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Custom email addresses are immutable and cannot be updated. Change email_address or domain_id to force replacement.",
	)
}

// Delete removes a custom email address.
func (r *EmailResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EmailResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/email/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete email %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete custom email",
			fmt.Sprintf("Could not delete email with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing custom email address by ID.
func (r *EmailResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// mapEmailAPIToModel maps an API email response to the Terraform resource model.
func mapEmailAPIToModel(email *apiEmail, model *EmailResourceModel) {
	model.ID = types.StringValue(email.ID)
	model.EmailAddress = types.StringValue(email.EmailAddress)
	model.DomainID = types.StringValue(email.DomainID)
	model.Active = types.BoolValue(email.Active)
	if email.SpaceID != "" {
		model.SpaceID = types.StringValue(email.SpaceID)
	} else {
		model.SpaceID = types.StringNull()
	}
}
