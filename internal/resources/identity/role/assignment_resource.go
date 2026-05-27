package role

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure AssignmentResource satisfies required interfaces.
var (
	_ resource.Resource                = &AssignmentResource{}
	_ resource.ResourceWithImportState = &AssignmentResource{}
)

// AssignmentResource implements the atlassian_role_assignment managed resource.
type AssignmentResource struct {
	client *atlassian.Client
}

// AssignmentResourceModel describes the resource data model for an Atlassian role assignment.
type AssignmentResourceModel struct {
	ID            types.String `tfsdk:"id"`
	RoleID        types.String `tfsdk:"role_id"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Scope         types.String `tfsdk:"scope"`
	ProductID     types.String `tfsdk:"product_id"`
}

// apiAssignmentRequest represents the JSON body for creating a role assignment.
type apiAssignmentRequest struct {
	RoleID        string `json:"roleId"`
	PrincipalType string `json:"principalType"`
	PrincipalID   string `json:"principalId"`
	Scope         string `json:"scope"`
	ProductID     string `json:"productId,omitempty"`
}

// apiAssignmentResponse represents the JSON response from the Atlassian role assignment API.
type apiAssignmentResponse struct {
	ID            string `json:"id"`
	RoleID        string `json:"roleId"`
	PrincipalType string `json:"principalType"`
	PrincipalID   string `json:"principalId"`
	Scope         string `json:"scope"`
	ProductID     string `json:"productId,omitempty"`
}

// principalTypeValidator validates that the principal_type attribute is either "user" or "group".
type principalTypeValidator struct{}

// Description returns a plain text description of the validator's behavior.
func (v principalTypeValidator) Description(_ context.Context) string {
	return "value must be \"user\" or \"group\""
}

// MarkdownDescription returns a markdown description of the validator's behavior.
func (v principalTypeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString validates that the string value is either "user" or "group".
func (v principalTypeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if value != "user" && value != "group" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid principal_type value",
			fmt.Sprintf("Principal type must be \"user\" or \"group\", got %q.", value),
		)
	}
}

// NewAssignmentResource returns a new instance of the role assignment resource.
func NewAssignmentResource() resource.Resource {
	return &AssignmentResource{}
}

// Metadata returns the resource type name.
func (r *AssignmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role_assignment"
}

// Schema defines the schema for the role assignment resource.
func (r *AssignmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Atlassian Cloud role assignment, binding a role to a user or group at the organization or product level.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the role assignment, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"role_id": schema.StringAttribute{
				Description: "The identifier of the role to assign.",
				Required:    true,
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal to assign the role to. Must be \"user\" or \"group\".",
				Required:    true,
				Validators: []validator.String{
					principalTypeValidator{},
				},
			},
			"principal_id": schema.StringAttribute{
				Description: "The identifier of the user or group to assign the role to.",
				Required:    true,
			},
			"scope": schema.StringAttribute{
				Description: "The scope of the role assignment. Must be \"org\" or \"product\".",
				Required:    true,
				Validators: []validator.String{
					scopeValidator{},
				},
			},
			"product_id": schema.StringAttribute{
				Description: "The product identifier. Required when scope is \"product\".",
				Optional:    true,
			},
		},
	}
}

// Configure retrieves the provider-configured client for API calls.
func (r *AssignmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new Atlassian role assignment.
func (r *AssignmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan AssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Scope.ValueString() == "product" && (plan.ProductID.IsNull() || plan.ProductID.IsUnknown() || plan.ProductID.ValueString() == "") {
		resp.Diagnostics.AddError(
			"Missing product_id",
			"The product_id attribute is required when scope is \"product\".",
		)
		return
	}

	reqBody := apiAssignmentRequest{
		RoleID:        plan.RoleID.ValueString(),
		PrincipalType: plan.PrincipalType.ValueString(),
		PrincipalID:   plan.PrincipalID.ValueString(),
		Scope:         plan.Scope.ValueString(),
		ProductID:     plan.ProductID.ValueString(),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	var apiResp apiAssignmentResponse
	err := r.client.Post(ctx, "/rest/api/3/role/assignment", bytes.NewReader(bodyBytes), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 400:
				resp.Diagnostics.AddError(
					"Invalid role assignment",
					fmt.Sprintf("The role %q could not be assigned. Verify the role ID is valid and the principal exists.", plan.RoleID.ValueString()),
				)
				return
			case 404:
				resp.Diagnostics.AddError(
					"Role or principal not found",
					fmt.Sprintf("The role %q or %s %q was not found. Verify both the role and the %s exist.",
						plan.RoleID.ValueString(), plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), plan.PrincipalType.ValueString()),
				)
				return
			case 409:
				resp.Diagnostics.AddError(
					"Duplicate role assignment",
					fmt.Sprintf("Role %q is already assigned to %s %q in scope %q. Each role can only be assigned once per principal and scope.",
						plan.RoleID.ValueString(), plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), plan.Scope.ValueString()),
				)
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to assign roles. Ensure the service account has organization admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create role assignment",
			fmt.Sprintf("Could not assign role %q to %s %q: %s",
				plan.RoleID.ValueString(), plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(apiResp.ID)
	plan.RoleID = types.StringValue(apiResp.RoleID)
	plan.PrincipalType = types.StringValue(apiResp.PrincipalType)
	plan.PrincipalID = types.StringValue(apiResp.PrincipalID)
	plan.Scope = types.StringValue(apiResp.Scope)
	if apiResp.ProductID != "" {
		plan.ProductID = types.StringValue(apiResp.ProductID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read retrieves the current state of an Atlassian role assignment.
func (r *AssignmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state AssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	assignmentID := state.ID.ValueString()
	var apiResp apiAssignmentResponse
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read role assignment",
			fmt.Sprintf("Role assignment with ID %s not found or could not be read: %s", assignmentID, err.Error()),
		)
		return
	}

	state.ID = types.StringValue(apiResp.ID)
	state.RoleID = types.StringValue(apiResp.RoleID)
	state.PrincipalType = types.StringValue(apiResp.PrincipalType)
	state.PrincipalID = types.StringValue(apiResp.PrincipalID)
	state.Scope = types.StringValue(apiResp.Scope)
	if apiResp.ProductID != "" {
		state.ProductID = types.StringValue(apiResp.ProductID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update replaces an existing Atlassian role assignment by deleting the old one and creating a new one.
func (r *AssignmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan AssignmentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state AssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Scope.ValueString() == "product" && (plan.ProductID.IsNull() || plan.ProductID.IsUnknown() || plan.ProductID.ValueString() == "") {
		resp.Diagnostics.AddError(
			"Missing product_id",
			"The product_id attribute is required when scope is \"product\".",
		)
		return
	}

	// Delete the existing assignment.
	oldID := state.ID.ValueString()
	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", oldID))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 404 {
			// Already deleted; continue with creation.
		} else {
			resp.Diagnostics.AddError(
				"Failed to delete old role assignment",
				fmt.Sprintf("Could not remove existing role assignment %s before replacement: %s", oldID, err.Error()),
			)
			return
		}
	}

	// Create the replacement assignment.
	reqBody := apiAssignmentRequest{
		RoleID:        plan.RoleID.ValueString(),
		PrincipalType: plan.PrincipalType.ValueString(),
		PrincipalID:   plan.PrincipalID.ValueString(),
		Scope:         plan.Scope.ValueString(),
		ProductID:     plan.ProductID.ValueString(),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	var apiResp apiAssignmentResponse
	err = r.client.Post(ctx, "/rest/api/3/role/assignment", bytes.NewReader(bodyBytes), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 400:
				resp.Diagnostics.AddError(
					"Invalid role assignment",
					fmt.Sprintf("The role %q could not be assigned. Verify the role ID is valid and the principal exists.", plan.RoleID.ValueString()),
				)
				return
			case 404:
				resp.Diagnostics.AddError(
					"Role or principal not found",
					fmt.Sprintf("The role %q or %s %q was not found. Verify both the role and the %s exist.",
						plan.RoleID.ValueString(), plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), plan.PrincipalType.ValueString()),
				)
				return
			case 409:
				resp.Diagnostics.AddError(
					"Duplicate role assignment",
					fmt.Sprintf("Role %q is already assigned to %s %q in scope %q. Each role can only be assigned once per principal and scope.",
						plan.RoleID.ValueString(), plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), plan.Scope.ValueString()),
				)
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to assign roles. Ensure the service account has organization admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update role assignment",
			fmt.Sprintf("Could not assign role %q to %s %q: %s",
				plan.RoleID.ValueString(), plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(apiResp.ID)
	plan.RoleID = types.StringValue(apiResp.RoleID)
	plan.PrincipalType = types.StringValue(apiResp.PrincipalType)
	plan.PrincipalID = types.StringValue(apiResp.PrincipalID)
	plan.Scope = types.StringValue(apiResp.Scope)
	if apiResp.ProductID != "" {
		plan.ProductID = types.StringValue(apiResp.ProductID)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an Atlassian role assignment.
func (r *AssignmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state AssignmentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	assignmentID := state.ID.ValueString()
	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/role/assignment/%s", assignmentID))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 404:
				// Assignment already deleted; nothing to do.
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to remove role assignments. "+
						"Ensure the service account has organization admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete role assignment",
			fmt.Sprintf("Could not remove role assignment %s: %s", assignmentID, err.Error()),
		)
	}
}

// ImportState imports an existing Atlassian role assignment by its composite ID.
// The composite ID format is "role_id/principal_type/principal_id/scope" or
// "role_id/principal_type/principal_id/scope/product_id" for product-scoped assignments.
func (r *AssignmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 4 && len(parts) != 5 {
		resp.Diagnostics.AddError(
			"Invalid import ID format",
			"Expected import ID format: role_id/principal_type/principal_id/scope or "+
				"role_id/principal_type/principal_id/scope/product_id for product-scoped assignments.",
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("role_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("principal_type"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("principal_id"), parts[2])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("scope"), parts[3])...)
	if len(parts) == 5 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("product_id"), parts[4])...)
	}
}
