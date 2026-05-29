// Package workflow implements the atlassian_jira_workflow managed resource.
//
// This resource manages Jira workflows through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via workflow ID.
package workflow

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

// apiWorkflowStatus represents a single status in a workflow.
type apiWorkflowStatus struct {
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
}

// apiWorkflowTransition represents a single transition in a workflow.
type apiWorkflowTransition struct {
	Name       string `json:"name"`
	FromStatus string `json:"fromStatus,omitempty"`
	ToStatus   string `json:"toStatus"`
}

// apiWorkflow represents the JSON structure returned by the Atlassian workflow API.
type apiWorkflow struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Self        string                  `json:"self"`
	Statuses    []apiWorkflowStatus     `json:"statuses,omitempty"`
	Transitions []apiWorkflowTransition `json:"transitions,omitempty"`
}

// apiWorkflowCreate represents the JSON body for creating a workflow.
type apiWorkflowCreate struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Statuses    []apiWorkflowStatus     `json:"statuses,omitempty"`
	Transitions []apiWorkflowTransition `json:"transitions,omitempty"`
}

// apiWorkflowUpdate represents the JSON body for updating a workflow.
type apiWorkflowUpdate struct {
	Name        string                  `json:"name,omitempty"`
	Description string                  `json:"description,omitempty"`
	Statuses    []apiWorkflowStatus     `json:"statuses,omitempty"`
	Transitions []apiWorkflowTransition `json:"transitions,omitempty"`
}

// StatusModel describes a single workflow status in the Terraform model.
type StatusModel struct {
	Name     string `tfsdk:"name"`
	Category string `tfsdk:"category"`
}

// TransitionModel describes a single workflow transition in the Terraform model.
type TransitionModel struct {
	Name       string `tfsdk:"name"`
	FromStatus string `tfsdk:"from_status"`
	ToStatus   string `tfsdk:"to_status"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Statuses    types.List   `tfsdk:"statuses"`
	Transitions types.List   `tfsdk:"transitions"`
}

// Resource implements the atlassian_jira_workflow managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow"
}

// Schema defines the schema for the jira workflow resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira workflow in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the workflow, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the workflow.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the workflow.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"statuses": schema.ListNestedAttribute{
				Description: "The statuses defined in the workflow.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the status.",
							Required:    true,
						},
						"category": schema.StringAttribute{
							Description: "The status category (e.g., \"new\", \"indeterminate\", \"done\").",
							Optional:    true,
						},
					},
				},
			},
			"transitions": schema.ListNestedAttribute{
				Description: "The transitions defined in the workflow.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the transition.",
							Required:    true,
						},
						"from_status": schema.StringAttribute{
							Description: "The source status name. Empty or omitted for the initial transition.",
							Optional:    true,
						},
						"to_status": schema.StringAttribute{
							Description: "The target status name.",
							Required:    true,
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

// Create provisions a new Jira workflow.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiWorkflowCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.Statuses = statusesFromPlan(ctx, plan.Statuses)
	body.Transitions = transitionsFromPlan(ctx, plan.Transitions)
	bodyBytes, _ := json.Marshal(body)

	var created apiWorkflow
	err := r.client.Post(ctx, "/rest/api/3/workflow", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate workflow name",
					fmt.Sprintf("A workflow with name %q already exists. Each workflow name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid workflow configuration",
					fmt.Sprintf("The workflow configuration for %q is invalid. Verify that all transitions, statuses, and conditions are correctly defined.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create workflows. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create workflow",
			fmt.Sprintf("Could not create workflow with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.Statuses = statusesToState(created.Statuses)
	plan.Transitions = transitionsToState(created.Transitions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var wf apiWorkflow
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", state.ID.ValueString()), &wf)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read workflow",
			fmt.Sprintf("Could not read workflow %q: %s. Verify the workflow exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(wf.ID)
	state.Name = types.StringValue(wf.Name)
	state.Description = types.StringValue(wf.Description)
	state.Statuses = statusesToState(wf.Statuses)
	state.Transitions = transitionsToState(wf.Transitions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira workflow.
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

	body := apiWorkflowUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.Statuses = statusesFromPlan(ctx, plan.Statuses)
	body.Transitions = transitionsFromPlan(ctx, plan.Transitions)
	bodyBytes, _ := json.Marshal(body)

	var updated apiWorkflow
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Workflow not found",
					fmt.Sprintf("Workflow with ID %q not found. The workflow may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid workflow configuration",
					fmt.Sprintf("The workflow update for ID %q is invalid. Verify that all transitions, statuses, and conditions are correctly defined.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update workflows. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update workflow",
			fmt.Sprintf("Could not update workflow with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.Statuses = statusesToState(updated.Statuses)
	plan.Transitions = transitionsToState(updated.Transitions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira workflow.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete workflow %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete workflow",
			fmt.Sprintf("Could not delete workflow with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing workflow by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// statusObjectType is the attr.Type for the status nested object.
var statusObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":     types.StringType,
		"category": types.StringType,
	},
}

// transitionObjectType is the attr.Type for the transition nested object.
var transitionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":        types.StringType,
		"from_status": types.StringType,
		"to_status":   types.StringType,
	},
}

// statusesFromPlan converts the Terraform statuses list to API format.
func statusesFromPlan(ctx context.Context, statuses types.List) []apiWorkflowStatus {
	if statuses.IsNull() || statuses.IsUnknown() {
		return nil
	}
	var models []StatusModel
	statuses.ElementsAs(ctx, &models, false)
	var result []apiWorkflowStatus
	for _, m := range models {
		result = append(result, apiWorkflowStatus{
			Name:     m.Name,
			Category: m.Category,
		})
	}
	return result
}

// transitionsFromPlan converts the Terraform transitions list to API format.
func transitionsFromPlan(ctx context.Context, transitions types.List) []apiWorkflowTransition {
	if transitions.IsNull() || transitions.IsUnknown() {
		return nil
	}
	var models []TransitionModel
	transitions.ElementsAs(ctx, &models, false)
	var result []apiWorkflowTransition
	for _, m := range models {
		result = append(result, apiWorkflowTransition{
			Name:       m.Name,
			FromStatus: m.FromStatus,
			ToStatus:   m.ToStatus,
		})
	}
	return result
}

// statusesToState converts API statuses to the Terraform state list.
func statusesToState(statuses []apiWorkflowStatus) types.List {
	if len(statuses) == 0 {
		return types.ListNull(statusObjectType)
	}
	var elems []attr.Value
	for _, s := range statuses {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"name":     types.StringType,
				"category": types.StringType,
			},
			map[string]attr.Value{
				"name":     types.StringValue(s.Name),
				"category": types.StringValue(s.Category),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(statusObjectType, elems)
	return list
}

// transitionsToState converts API transitions to the Terraform state list.
func transitionsToState(transitions []apiWorkflowTransition) types.List {
	if len(transitions) == 0 {
		return types.ListNull(transitionObjectType)
	}
	var elems []attr.Value
	for _, t := range transitions {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"name":        types.StringType,
				"from_status": types.StringType,
				"to_status":   types.StringType,
			},
			map[string]attr.Value{
				"name":        types.StringValue(t.Name),
				"from_status": types.StringValue(t.FromStatus),
				"to_status":   types.StringValue(t.ToStatus),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(transitionObjectType, elems)
	return list
}
