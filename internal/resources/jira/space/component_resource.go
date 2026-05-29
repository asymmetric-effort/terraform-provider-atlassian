// Package space implements Jira space (project) resources including
// project components.
//
// ComponentResource manages Jira project components through the Atlassian
// Cloud REST API (/rest/api/3/component). It supports full CRUD operations
// and state import via component ID.
package space

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

// Ensure ComponentResource satisfies framework interfaces.
var (
	_ resource.Resource                = &ComponentResource{}
	_ resource.ResourceWithImportState = &ComponentResource{}
)

// apiComponent represents the JSON structure returned by the Atlassian component API.
type apiComponent struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	LeadAccountID string `json:"leadAccountId,omitempty"`
	AssigneeType  string `json:"assigneeType,omitempty"`
	Project       string `json:"project,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
	Self          string `json:"self,omitempty"`
}

// apiComponentCreate represents the JSON body for creating a component.
type apiComponentCreate struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	LeadAccountID string `json:"leadAccountId,omitempty"`
	AssigneeType  string `json:"assigneeType,omitempty"`
	Project       string `json:"project,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
}

// apiComponentUpdate represents the JSON body for updating a component.
type apiComponentUpdate struct {
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	LeadAccountID string `json:"leadAccountId,omitempty"`
	AssigneeType  string `json:"assigneeType,omitempty"`
}

// ComponentResourceModel describes the component resource data model.
type ComponentResourceModel struct {
	ID            types.String `tfsdk:"id"`
	SpaceID       types.String `tfsdk:"space_id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	LeadAccountID types.String `tfsdk:"lead_account_id"`
	AssigneeType  types.String `tfsdk:"assignee_type"`
}

// ComponentResource implements the atlassian_jira_project_component managed resource.
type ComponentResource struct {
	client *atlassian.Client
}

// NewComponentResource returns a new ComponentResource instance for provider registration.
func NewComponentResource() resource.Resource {
	return &ComponentResource{}
}

// Metadata returns the resource type name.
func (r *ComponentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_component"
}

// Schema defines the schema for the jira project component resource.
func (r *ComponentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira project component in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the component, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The project ID that the component belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the component. Must be unique within the project.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the component.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"lead_account_id": schema.StringAttribute{
				Description: "The account ID of the component lead.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"assignee_type": schema.StringAttribute{
				Description: "The assignee type for issues in this component. Valid values: PROJECT_DEFAULT, COMPONENT_LEAD, PROJECT_LEAD, UNASSIGNED.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *ComponentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira project component.
func (r *ComponentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ComponentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiComponentCreate{
		Name:      plan.Name.ValueString(),
		ProjectID: plan.SpaceID.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.LeadAccountID.IsNull() && !plan.LeadAccountID.IsUnknown() {
		body.LeadAccountID = plan.LeadAccountID.ValueString()
	}
	if !plan.AssigneeType.IsNull() && !plan.AssigneeType.IsUnknown() {
		body.AssigneeType = plan.AssigneeType.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiComponent
	err := r.client.Post(ctx, "/rest/api/3/component", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate component name",
					fmt.Sprintf("A component with name %q already exists in project %q. Each component name must be unique within a project.",
						plan.Name.ValueString(), plan.SpaceID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create components. Ensure the service account has project admin privileges.",
				)
				return
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Project not found",
					fmt.Sprintf("Project with ID %q not found. Verify the space_id is correct.", plan.SpaceID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create component",
			fmt.Sprintf("Could not create component %q in project %q: %s",
				plan.Name.ValueString(), plan.SpaceID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.LeadAccountID = types.StringValue(created.LeadAccountID)
	plan.AssigneeType = types.StringValue(created.AssigneeType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *ComponentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ComponentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var component apiComponent
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/component/%s", state.ID.ValueString()), &component)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read component",
			fmt.Sprintf("Could not read component %q: %s. Verify the component exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(component.ID)
	state.Name = types.StringValue(component.Name)
	state.Description = types.StringValue(component.Description)
	state.LeadAccountID = types.StringValue(component.LeadAccountID)
	state.AssigneeType = types.StringValue(component.AssigneeType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira project component.
func (r *ComponentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ComponentResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ComponentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiComponentUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.LeadAccountID.IsNull() && !plan.LeadAccountID.IsUnknown() {
		body.LeadAccountID = plan.LeadAccountID.ValueString()
	}
	if !plan.AssigneeType.IsNull() && !plan.AssigneeType.IsUnknown() {
		body.AssigneeType = plan.AssigneeType.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiComponent
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/component/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Component not found",
					fmt.Sprintf("Component with ID %q not found. The component may have been deleted outside of Terraform.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update components. Ensure the service account has project admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update component",
			fmt.Sprintf("Could not update component with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.LeadAccountID = types.StringValue(updated.LeadAccountID)
	plan.AssigneeType = types.StringValue(updated.AssigneeType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira project component.
func (r *ComponentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ComponentResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/component/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete component %q. "+
						"Ensure the service account has project admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete component",
			fmt.Sprintf("Could not delete component with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing component by ID.
func (r *ComponentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
