// Package role implements the atlassian_role managed resource.
//
// This resource manages Atlassian Cloud project roles via the REST API.
// It supports full CRUD operations and ImportState for tofu import.
package role

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure Resource satisfies required interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// Resource implements the atlassian_role managed resource.
type Resource struct {
	client *atlassian.Client
}

// ResourceModel describes the resource data model for an Atlassian role.
type ResourceModel struct {
	RoleID      types.String `tfsdk:"role_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Scope       types.String `tfsdk:"scope"`
}

// apiRoleResponse represents the JSON response from the Atlassian role API.
type apiRoleResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope,omitempty"`
}

// apiRoleCreateRequest represents the JSON body for creating a role.
type apiRoleCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// apiRoleUpdateRequest represents the JSON body for updating a role.
type apiRoleUpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// scopeValidator validates that the scope attribute is either "org" or "product".
type scopeValidator struct{}

// Description returns a plain text description of the validator's behavior.
func (v scopeValidator) Description(_ context.Context) string {
	return "value must be \"org\" or \"product\""
}

// MarkdownDescription returns a markdown description of the validator's behavior.
func (v scopeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

// ValidateString validates that the string value is either "org" or "product".
func (v scopeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if value != "org" && value != "product" {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid scope value",
			fmt.Sprintf("Scope must be \"org\" or \"product\", got %q.", value),
		)
	}
}

// NewResource returns a new instance of the role resource.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

// Schema defines the schema for the role resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Atlassian Cloud role.",
		Attributes: map[string]schema.Attribute{
			"role_id": schema.StringAttribute{
				Description: "The unique identifier of the role, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the role. Must be unique within the Atlassian organization.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the role.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"scope": schema.StringAttribute{
				Description: "The scope of the role. Must be \"org\" or \"product\".",
				Required:    true,
				Validators: []validator.String{
					scopeValidator{},
				},
			},
		},
	}
}

// Configure retrieves the provider-configured client for API calls.
func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates a new Atlassian role.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	reqBody := apiRoleCreateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	var apiResp apiRoleResponse
	err := r.client.Post(ctx, "/rest/api/3/role", bytes.NewReader(bodyBytes), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 409:
				resp.Diagnostics.AddError(
					"Duplicate role name",
					fmt.Sprintf("A role with the name %q already exists. Each role name must be unique.", plan.Name.ValueString()),
				)
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create roles. Ensure the service account has organization admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create role",
			"Could not create role '"+plan.Name.ValueString()+"': "+err.Error(),
		)
		return
	}

	plan.RoleID = types.StringValue(fmt.Sprintf("%d", apiResp.ID))
	plan.Name = types.StringValue(apiResp.Name)
	plan.Description = types.StringValue(apiResp.Description)
	if apiResp.Scope != "" {
		plan.Scope = types.StringValue(apiResp.Scope)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read retrieves the current state of an Atlassian role.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleID := state.RoleID.ValueString()
	var apiResp apiRoleResponse
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read role",
			fmt.Sprintf("Role with ID %s not found or could not be read: %s", roleID, err.Error()),
		)
		return
	}

	state.RoleID = types.StringValue(fmt.Sprintf("%d", apiResp.ID))
	state.Name = types.StringValue(apiResp.Name)
	state.Description = types.StringValue(apiResp.Description)
	if apiResp.Scope != "" {
		state.Scope = types.StringValue(apiResp.Scope)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Atlassian role.
func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleID := state.RoleID.ValueString()
	reqBody := apiRoleUpdateRequest{
		Name:        plan.Name.ValueString(),
		Description: plan.Description.ValueString(),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	var apiResp apiRoleResponse
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), bytes.NewReader(bodyBytes), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 404:
				resp.Diagnostics.AddError(
					"Role not found",
					fmt.Sprintf("Role with ID %s not found. The role may have been deleted outside of Terraform.", roleID),
				)
				return
			case 409:
				resp.Diagnostics.AddError(
					"Duplicate role name",
					fmt.Sprintf("A role with the name %q already exists. Each role name must be unique.", plan.Name.ValueString()),
				)
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update roles. Ensure the service account has organization admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update role",
			"Could not update role '"+plan.Name.ValueString()+"' (ID: "+roleID+"): "+err.Error(),
		)
		return
	}

	plan.RoleID = types.StringValue(fmt.Sprintf("%d", apiResp.ID))
	plan.Name = types.StringValue(apiResp.Name)
	plan.Description = types.StringValue(apiResp.Description)
	if apiResp.Scope != "" {
		plan.Scope = types.StringValue(apiResp.Scope)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an Atlassian role.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleID := state.RoleID.ValueString()
	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 404:
				// Role already deleted; nothing to do.
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to delete role '"+state.Name.ValueString()+"'. "+
						"Ensure the service account has organization admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete role",
			"Could not delete role '"+state.Name.ValueString()+"' (ID: "+roleID+"): "+err.Error(),
		)
	}
}

// ImportState imports an existing Atlassian role by its role ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("role_id"), req, resp)
}
