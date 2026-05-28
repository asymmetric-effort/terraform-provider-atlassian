// Package page implements the atlassian_statuspage_permission managed resource.
package page

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the PermissionResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &PermissionResource{}
	_ resource.ResourceWithImportState = &PermissionResource{}
)

// apiPermissionSP represents the JSON structure for Statuspage permissions.
type apiPermissionSP struct {
	ID            string `json:"id"`
	PageID        string `json:"page_id"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Role          string `json:"role"`
}

// apiPermissionSPCreate represents the JSON body for creating a permission.
type apiPermissionSPCreate struct {
	Permission apiPermissionSPBody `json:"permission"`
}

// apiPermissionSPBody holds the permission fields.
type apiPermissionSPBody struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Role          string `json:"role"`
}

// PermissionResourceModel describes the resource data model.
type PermissionResourceModel struct {
	ID            types.String `tfsdk:"id"`
	PageID        types.String `tfsdk:"page_id"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Role          types.String `tfsdk:"role"`
}

// PermissionResource implements the atlassian_statuspage_permission managed resource.
type PermissionResource struct {
	client *atlassian.Client
}

// NewPermissionResource returns a new PermissionResource instance for provider registration.
func NewPermissionResource() resource.Resource {
	return &PermissionResource{}
}

// Metadata returns the resource type name.
func (r *PermissionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_permission"
}

// Schema defines the schema for the Statuspage permission resource.
func (r *PermissionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Statuspage page permission (team member access).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the permission.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
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
				Description: "The ID of the principal (user or group).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"role": schema.StringAttribute{
				Description: "The role assigned. Must be \"admin\", \"member\", or \"viewer\".",
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

// Create provisions a new Statuspage permission.
func (r *PermissionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PermissionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiPermissionSPCreate{
		Permission: apiPermissionSPBody{
			PrincipalType: plan.PrincipalType.ValueString(),
			PrincipalID:   plan.PrincipalID.ValueString(),
			Role:          plan.Role.ValueString(),
		},
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiPermissionSP
	err := r.client.Post(ctx, fmt.Sprintf("/v1/pages/%s/permissions", plan.PageID.ValueString()), bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate permission",
					fmt.Sprintf("A permission for %s %q already exists on page %q.",
						plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), plan.PageID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to manage Statuspage page permissions. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create Statuspage permission",
			fmt.Sprintf("Could not create permission for %s %q on page %q: %s",
				plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), plan.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.PageID = types.StringValue(created.PageID)
	plan.PrincipalType = types.StringValue(created.PrincipalType)
	plan.PrincipalID = types.StringValue(created.PrincipalID)
	plan.Role = types.StringValue(created.Role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *PermissionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var perm apiPermissionSP
	err := r.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/permissions/%s", state.PageID.ValueString(), state.ID.ValueString()), &perm)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage permission",
			fmt.Sprintf("Could not read permission %q on page %q: %s. Verify the permission exists and has not been revoked.",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(perm.ID)
	state.PageID = types.StringValue(perm.PageID)
	state.PrincipalType = types.StringValue(perm.PrincipalType)
	state.PrincipalID = types.StringValue(perm.PrincipalID)
	state.Role = types.StringValue(perm.Role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing permission (role change).
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

	body := apiPermissionSPCreate{
		Permission: apiPermissionSPBody{
			PrincipalType: plan.PrincipalType.ValueString(),
			PrincipalID:   plan.PrincipalID.ValueString(),
			Role:          plan.Role.ValueString(),
		},
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiPermissionSP
	err := r.client.Put(ctx, fmt.Sprintf("/v1/pages/%s/permissions/%s", state.PageID.ValueString(), state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Permission not found",
					fmt.Sprintf("Permission %q on page %q not found. The permission may have been revoked outside of Terraform.",
						state.ID.ValueString(), state.PageID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to manage Statuspage page permissions. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Statuspage permission",
			fmt.Sprintf("Could not update permission %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.PageID = types.StringValue(updated.PageID)
	plan.PrincipalType = types.StringValue(updated.PrincipalType)
	plan.PrincipalID = types.StringValue(updated.PrincipalID)
	plan.Role = types.StringValue(updated.Role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Statuspage permission.
func (r *PermissionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PermissionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/v1/pages/%s/permissions/%s", state.PageID.ValueString(), state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to revoke permission %q on page %q. "+
						"Ensure the service account has Statuspage admin privileges.", state.ID.ValueString(), state.PageID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Statuspage permission",
			fmt.Sprintf("Could not delete permission %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing permission by ID.
func (r *PermissionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
