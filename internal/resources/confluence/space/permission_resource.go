// Package space implements the atlassian_confluence_space_permission managed resource.
//
// This resource manages space permissions in Confluence through the Atlassian
// Cloud REST API (v2). It supports full CRUD operations and state import via
// composite ID.
package space

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

// apiPermission represents the JSON structure returned by the space permission API.
type apiPermission struct {
	ID        string             `json:"id"`
	Principal apiPermissionPrinc `json:"principal"`
	Operation apiPermissionOp    `json:"operation"`
}

// apiPermissionPrinc represents the principal in a permission.
type apiPermissionPrinc struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// apiPermissionOp represents the operation in a permission.
type apiPermissionOp struct {
	Key string `json:"key"`
}

// apiPermissionCreate represents the JSON body for creating a permission.
type apiPermissionCreate struct {
	Principal apiPermissionPrinc `json:"principal"`
	Operation apiPermissionOp    `json:"operation"`
}

// PermissionResourceModel describes the resource data model.
type PermissionResourceModel struct {
	ID            types.String `tfsdk:"id"`
	SpaceID       types.String `tfsdk:"space_id"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Operation     types.String `tfsdk:"operation"`
}

// PermissionResource implements the atlassian_confluence_space_permission managed resource.
type PermissionResource struct {
	client *atlassian.Client
}

// NewPermissionResource returns a new PermissionResource instance for provider registration.
func NewPermissionResource() resource.Resource {
	return &PermissionResource{}
}

// Metadata returns the resource type name.
func (r *PermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_space_permission"
}

// Schema defines the schema for the space permission resource.
func (r *PermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a space permission in Confluence Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the permission, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the Confluence space.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal. Must be \"user\" or \"group\".",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_id": schema.StringAttribute{
				Description: "The ID of the principal (user account ID or group ID).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"operation": schema.StringAttribute{
				Description: "The operation to grant. Must be \"read\", \"write\", or \"admin\".",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
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

// compositePermissionID builds a composite ID from space ID and permission ID.
func compositePermissionID(spaceID, permID string) string {
	return fmt.Sprintf("%s/%s", spaceID, permID)
}

// parsePermissionID parses a composite permission ID into space ID and permission ID.
func parsePermissionID(id string) (spaceID, permID string, err error) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected format: space_id/permission_id, got %q", id)
	}
	return parts[0], parts[1], nil
}

// Create provisions a new space permission.
func (r *PermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiPermissionCreate{
		Principal: apiPermissionPrinc{
			Type: plan.PrincipalType.ValueString(),
			ID:   plan.PrincipalID.ValueString(),
		},
		Operation: apiPermissionOp{
			Key: plan.Operation.ValueString(),
		},
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiPermission
	err := r.client.Post(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", plan.SpaceID.ValueString()), bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate permission",
					fmt.Sprintf("Permission already exists for principal %q on space %q.", plan.PrincipalID.ValueString(), plan.SpaceID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to manage space permissions.",
				)
				return
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Space not found",
					fmt.Sprintf("Space %q not found.", plan.SpaceID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create space permission",
			fmt.Sprintf("Could not create permission on space %q: %s", plan.SpaceID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(compositePermissionID(plan.SpaceID.ValueString(), created.ID))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *PermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spaceID := state.SpaceID.ValueString()

	var permissions []apiPermission
	err := r.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &permissions)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read space permissions",
			fmt.Sprintf("Could not read permissions for space %q: %s", spaceID, err.Error()),
		)
		return
	}

	found := false
	for _, perm := range permissions {
		if perm.Principal.Type == state.PrincipalType.ValueString() &&
			perm.Principal.ID == state.PrincipalID.ValueString() &&
			perm.Operation.Key == state.Operation.ValueString() {
			state.ID = types.StringValue(compositePermissionID(spaceID, perm.ID))
			found = true
			break
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing space permission (replace-in-place via delete+create).
func (r *PermissionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Space permissions are immutable. Changes require replacement.",
	)
}

// Delete removes a space permission.
func (r *PermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Extract the permission ID from the composite ID
	_, permID, parseErr := parsePermissionID(state.ID.ValueString())
	if parseErr != nil {
		resp.Diagnostics.AddError(
			"Invalid permission ID",
			fmt.Sprintf("Could not parse permission ID: %s", parseErr.Error()),
		)
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions/%s", state.SpaceID.ValueString(), permID))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete permissions on space %q.", state.SpaceID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete space permission",
			fmt.Sprintf("Could not delete permission on space %q: %s", state.SpaceID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing space permission by composite ID (space_id/permission_id).
func (r *PermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	_, _, err := parsePermissionID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format: space_id/permission_id. Error: %s", err.Error()),
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
