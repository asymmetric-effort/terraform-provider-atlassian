// Package permissionscheme implements the atlassian_jira_permission_scheme managed resource.
//
// This resource manages Jira permission schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via permission scheme ID.
package permissionscheme

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

// apiPermissionGrant represents a single permission grant in a scheme.
type apiPermissionGrant struct {
	Permission string              `json:"permission"`
	Holder     apiPermissionHolder `json:"holder"`
}

// apiPermissionHolder represents who holds a permission.
type apiPermissionHolder struct {
	Type      string `json:"type"`
	Parameter string `json:"parameter,omitempty"`
}

// apiPermissionScheme represents the JSON structure returned by the Atlassian permission scheme API.
type apiPermissionScheme struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Self        string               `json:"self"`
	Permissions []apiPermissionGrant `json:"permissions,omitempty"`
}

// apiPermissionSchemeCreate represents the JSON body for creating a permission scheme.
type apiPermissionSchemeCreate struct {
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Permissions []apiPermissionGrant `json:"permissions,omitempty"`
}

// apiPermissionSchemeUpdate represents the JSON body for updating a permission scheme.
type apiPermissionSchemeUpdate struct {
	Name        string               `json:"name,omitempty"`
	Description string               `json:"description,omitempty"`
	Permissions []apiPermissionGrant `json:"permissions,omitempty"`
}

// GrantModel describes a single permission grant in the Terraform model.
type GrantModel struct {
	Permission string `tfsdk:"permission"`
	HolderType string `tfsdk:"holder_type"`
	HolderID   string `tfsdk:"holder_id"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Grants      types.List   `tfsdk:"grants"`
}

// Resource implements the atlassian_jira_permission_scheme managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_permission_scheme"
}

// Schema defines the schema for the jira permission scheme resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira permission scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the permission scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the permission scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the permission scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"grants": schema.ListNestedAttribute{
				Description: "Permission grants mapping permissions to holders (users, groups, roles).",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"permission": schema.StringAttribute{
							Description: "The permission key (e.g., BROWSE_PROJECTS, EDIT_ISSUES).",
							Required:    true,
						},
						"holder_type": schema.StringAttribute{
							Description: "The type of holder: user, group, projectRole, or projectLead.",
							Required:    true,
						},
						"holder_id": schema.StringAttribute{
							Description: "The ID of the holder (user account ID, group name, or role ID). Empty for projectLead.",
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

// Create provisions a new Jira permission scheme.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiPermissionSchemeCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.Permissions = grantsFromPlan(ctx, plan.Grants)
	bodyBytes, _ := json.Marshal(body)

	var created apiPermissionScheme
	err := r.client.Post(ctx, "/rest/api/3/permissionscheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate permission scheme name",
					fmt.Sprintf("A permission scheme with name %q already exists. Each permission scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid permission scheme configuration",
					fmt.Sprintf("The permission scheme configuration for %q is invalid. Verify the scheme name and permissions are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create permission schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create permission scheme",
			fmt.Sprintf("Could not create permission scheme with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.Grants = grantsToState(ctx, created.Permissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ps apiPermissionScheme
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/permissionscheme/%s", state.ID.ValueString()), &ps)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read permission scheme",
			fmt.Sprintf("Could not read permission scheme %q: %s. Verify the permission scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(ps.ID)
	state.Name = types.StringValue(ps.Name)
	state.Description = types.StringValue(ps.Description)
	state.Grants = grantsToState(ctx, ps.Permissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira permission scheme.
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

	body := apiPermissionSchemeUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.Permissions = grantsFromPlan(ctx, plan.Grants)
	bodyBytes, _ := json.Marshal(body)

	var updated apiPermissionScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/permissionscheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Permission scheme not found",
					fmt.Sprintf("Permission scheme with ID %q not found. The permission scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid permission scheme configuration",
					fmt.Sprintf("The permission scheme update for ID %q is invalid. Verify the scheme name and permissions are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update permission schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update permission scheme",
			fmt.Sprintf("Could not update permission scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.Grants = grantsToState(ctx, updated.Permissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira permission scheme.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/permissionscheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete permission scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete permission scheme",
			fmt.Sprintf("Could not delete permission scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing permission scheme by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// grantObjectType is the attr.Type for the grant nested object.
var grantObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"permission":  types.StringType,
		"holder_type": types.StringType,
		"holder_id":   types.StringType,
	},
}

// grantsFromPlan converts the Terraform grants list to API format.
func grantsFromPlan(ctx context.Context, grants types.List) []apiPermissionGrant {
	if grants.IsNull() || grants.IsUnknown() {
		return nil
	}
	var grantModels []GrantModel
	grants.ElementsAs(ctx, &grantModels, false)
	var result []apiPermissionGrant
	for _, g := range grantModels {
		result = append(result, apiPermissionGrant{
			Permission: g.Permission,
			Holder: apiPermissionHolder{
				Type:      g.HolderType,
				Parameter: g.HolderID,
			},
		})
	}
	return result
}

// grantsToState converts API grants to the Terraform state list.
func grantsToState(ctx context.Context, perms []apiPermissionGrant) types.List {
	if len(perms) == 0 {
		return types.ListNull(grantObjectType)
	}
	var elems []attr.Value
	for _, p := range perms {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"permission":  types.StringType,
				"holder_type": types.StringType,
				"holder_id":   types.StringType,
			},
			map[string]attr.Value{
				"permission":  types.StringValue(p.Permission),
				"holder_type": types.StringValue(p.Holder.Type),
				"holder_id":   types.StringValue(p.Holder.Parameter),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(grantObjectType, elems)
	return list
}
