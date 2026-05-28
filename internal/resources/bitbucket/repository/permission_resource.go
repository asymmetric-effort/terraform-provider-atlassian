// Package repository implements the atlassian_bitbucket_repository_permission managed resource.
//
// This resource manages Bitbucket repository permissions through the Atlassian
// Cloud REST API. It supports full CRUD operations and state import
// via permission ID.
package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the PermissionResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &PermissionResource{}
	_ resource.ResourceWithImportState = &PermissionResource{}
)

// apiPermission represents the JSON structure returned by the Bitbucket permissions-config API.
type apiPermission struct {
	ID            string `json:"id"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Permission    string `json:"permission"`
}

// apiPermissionCreate represents the JSON body for creating a repository permission.
type apiPermissionCreate struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Permission    string `json:"permission"`
}

// PermissionResourceModel describes the resource data model.
type PermissionResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Repository    types.String `tfsdk:"repository"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Permission    types.String `tfsdk:"permission"`
}

// PermissionResource implements the atlassian_bitbucket_repository_permission managed resource.
type PermissionResource struct {
	client *atlassian.Client
}

// NewPermissionResource returns a new PermissionResource instance for provider registration.
func NewPermissionResource() resource.Resource {
	return &PermissionResource{}
}

// Metadata returns the resource type name.
func (r *PermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_repository_permission"
}

// Schema defines the schema for the bitbucket repository permission resource.
func (r *PermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Bitbucket repository permission in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the permission, assigned by Bitbucket.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"repository": schema.StringAttribute{
				Description: "The repository in workspace/slug format (e.g., myworkspace/myrepo).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal. Must be one of: user, group.",
				Required:    true,
			},
			"principal_id": schema.StringAttribute{
				Description: "The unique identifier of the principal (user account ID or group slug).",
				Required:    true,
			},
			"permission": schema.StringAttribute{
				Description: "The permission level. Must be one of: read, write, admin.",
				Required:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *PermissionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// repoPath builds the API path prefix for a repository.
func permRepoPath(repository string) string {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("/2.0/repositories/%s/%s", parts[0], parts[1])
}

// Create provisions a new Bitbucket repository permission.
func (r *PermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := permRepoPath(plan.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError(
			"Invalid repository format",
			"Repository must be in workspace/slug format (e.g., myworkspace/myrepo).",
		)
		return
	}

	body := apiPermissionCreate{
		PrincipalType: plan.PrincipalType.ValueString(),
		PrincipalID:   plan.PrincipalID.ValueString(),
		Permission:    plan.Permission.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiPermission
	err := r.client.Post(ctx, base+"/permissions-config", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to manage repository permissions.",
				)
				return
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Repository not found",
					fmt.Sprintf("Repository %q not found. Verify the workspace/slug is correct.", plan.Repository.ValueString()),
				)
				return
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate permission",
					"A permission for this principal already exists in this repository.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create repository permission",
			fmt.Sprintf("Could not create repository permission: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.PrincipalType = types.StringValue(created.PrincipalType)
	plan.PrincipalID = types.StringValue(created.PrincipalID)
	plan.Permission = types.StringValue(created.Permission)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Bitbucket.
func (r *PermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := permRepoPath(state.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError("Invalid repository format", "Repository must be in workspace/slug format.")
		return
	}

	var perm apiPermission
	err := r.client.Get(ctx, fmt.Sprintf("%s/permissions-config/%s", base, state.ID.ValueString()), &perm)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read repository permission",
			fmt.Sprintf("Could not read repository permission %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(perm.ID)
	state.PrincipalType = types.StringValue(perm.PrincipalType)
	state.PrincipalID = types.StringValue(perm.PrincipalID)
	state.Permission = types.StringValue(perm.Permission)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Bitbucket repository permission.
func (r *PermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan PermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := permRepoPath(state.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError("Invalid repository format", "Repository must be in workspace/slug format.")
		return
	}

	body := apiPermissionCreate{
		PrincipalType: plan.PrincipalType.ValueString(),
		PrincipalID:   plan.PrincipalID.ValueString(),
		Permission:    plan.Permission.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiPermission
	err := r.client.Put(ctx, fmt.Sprintf("%s/permissions-config/%s", base, state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Permission not found",
					fmt.Sprintf("Repository permission %q not found. It may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update repository permissions.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update repository permission",
			fmt.Sprintf("Could not update repository permission %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.PrincipalType = types.StringValue(updated.PrincipalType)
	plan.PrincipalID = types.StringValue(updated.PrincipalID)
	plan.Permission = types.StringValue(updated.Permission)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Bitbucket repository permission.
func (r *PermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := permRepoPath(state.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError("Invalid repository format", "Repository must be in workspace/slug format.")
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("%s/permissions-config/%s", base, state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to delete repository permissions.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete repository permission",
			fmt.Sprintf("Could not delete repository permission %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing repository permission by ID.
func (r *PermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
