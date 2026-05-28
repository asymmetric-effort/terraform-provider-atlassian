// Package dashboard implements the atlassian_jira_dashboard managed resource.
//
// This resource manages Jira dashboards through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via dashboard ID.
package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiSharePermission represents a share permission entry in the Atlassian API.
type apiSharePermission struct {
	Type      string `json:"type"`
	Parameter string `json:"parameter,omitempty"`
}

// apiDashboard represents the JSON structure returned by the Atlassian dashboard API.
type apiDashboard struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	Self             string               `json:"self"`
	SharePermissions []apiSharePermission `json:"sharePermissions,omitempty"`
}

// apiDashboardCreate represents the JSON body for creating a dashboard.
type apiDashboardCreate struct {
	Name             string               `json:"name"`
	Description      string               `json:"description,omitempty"`
	SharePermissions []apiSharePermission `json:"sharePermissions,omitempty"`
}

// apiDashboardUpdate represents the JSON body for updating a dashboard.
type apiDashboardUpdate struct {
	Name             string               `json:"name,omitempty"`
	Description      string               `json:"description,omitempty"`
	SharePermissions []apiSharePermission `json:"sharePermissions,omitempty"`
}

// SharePermissionModel describes a single share permission in the Terraform model.
type SharePermissionModel struct {
	Type      string `tfsdk:"type"`
	Parameter string `tfsdk:"parameter"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	SharePermissions types.List   `tfsdk:"share_permissions"`
}

// sharePermissionObjectType is the attr.Type for the share permission nested object.
var sharePermissionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"type":      types.StringType,
		"parameter": types.StringType,
	},
}

// sharePermissionsFromPlan converts the Terraform share_permissions list to API format.
func sharePermissionsFromPlan(ctx context.Context, perms types.List) []apiSharePermission {
	if perms.IsNull() || perms.IsUnknown() {
		return nil
	}
	var models []SharePermissionModel
	perms.ElementsAs(ctx, &models, false)
	var result []apiSharePermission
	for _, m := range models {
		result = append(result, apiSharePermission{
			Type:      m.Type,
			Parameter: m.Parameter,
		})
	}
	return result
}

// sharePermissionsToState converts API share permissions to the Terraform state list.
func sharePermissionsToState(ctx context.Context, perms []apiSharePermission) types.List {
	if len(perms) == 0 {
		return types.ListNull(sharePermissionObjectType)
	}
	var elems []attr.Value
	for _, p := range perms {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"type":      types.StringType,
				"parameter": types.StringType,
			},
			map[string]attr.Value{
				"type":      types.StringValue(p.Type),
				"parameter": types.StringValue(p.Parameter),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(sharePermissionObjectType, elems)
	return list
}

// Resource implements the atlassian_jira_dashboard managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_dashboard"
}

// Schema defines the schema for the jira dashboard resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira dashboard in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the dashboard, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the dashboard.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the dashboard.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"share_permissions": schema.ListNestedAttribute{
				Description: "Share permissions controlling who can view the dashboard (e.g., global, project, group).",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Description: "The type of share permission: global, project, or group.",
							Required:    true,
						},
						"parameter": schema.StringAttribute{
							Description: "The parameter for the share permission (e.g., group name). Empty for global.",
							Optional:    true,
						},
					},
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
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

// Create provisions a new Jira dashboard.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiDashboardCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.SharePermissions = sharePermissionsFromPlan(ctx, plan.SharePermissions)
	bodyBytes, _ := json.Marshal(body)

	var created apiDashboard
	err := r.client.Post(ctx, "/rest/api/3/dashboard", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate dashboard name",
					fmt.Sprintf("A dashboard with name %q already exists. Each dashboard name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid dashboard configuration",
					fmt.Sprintf("The dashboard configuration for %q is invalid. Verify the dashboard name and description are valid.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create dashboards. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create dashboard",
			fmt.Sprintf("Could not create dashboard with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.SharePermissions = sharePermissionsToState(ctx, created.SharePermissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var d apiDashboard
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/dashboard/%s", state.ID.ValueString()), &d)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read dashboard",
			fmt.Sprintf("Could not read dashboard %q: %s. Verify the dashboard exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(d.ID)
	state.Name = types.StringValue(d.Name)
	state.Description = types.StringValue(d.Description)
	state.SharePermissions = sharePermissionsToState(ctx, d.SharePermissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira dashboard.
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

	body := apiDashboardUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.SharePermissions = sharePermissionsFromPlan(ctx, plan.SharePermissions)
	bodyBytes, _ := json.Marshal(body)

	var updated apiDashboard
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/dashboard/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Dashboard not found",
					fmt.Sprintf("Dashboard with ID %q not found. The dashboard may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid dashboard configuration",
					fmt.Sprintf("The dashboard update for ID %q is invalid. Verify the dashboard name and description are valid.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update dashboards. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update dashboard",
			fmt.Sprintf("Could not update dashboard with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.SharePermissions = sharePermissionsToState(ctx, updated.SharePermissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira dashboard.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/dashboard/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete dashboard %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete dashboard",
			fmt.Sprintf("Could not delete dashboard with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing dashboard by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
