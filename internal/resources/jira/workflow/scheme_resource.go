// Package workflow implements the atlassian_jira_workflow_scheme managed resource.
//
// This resource manages Jira workflow schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via workflow scheme ID.
package workflow

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

// Ensure the SchemeResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &SchemeResource{}
	_ resource.ResourceWithImportState = &SchemeResource{}
)

// apiWorkflowScheme represents the JSON structure returned by the Atlassian workflow scheme API.
type apiWorkflowScheme struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	DefaultWorkflowID string `json:"defaultWorkflow,omitempty"`
	Self              string `json:"self"`
}

// apiWorkflowSchemeCreate represents the JSON body for creating a workflow scheme.
type apiWorkflowSchemeCreate struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	DefaultWorkflowID string `json:"defaultWorkflow,omitempty"`
}

// apiWorkflowSchemeUpdate represents the JSON body for updating a workflow scheme.
type apiWorkflowSchemeUpdate struct {
	Name              string `json:"name,omitempty"`
	Description       string `json:"description,omitempty"`
	DefaultWorkflowID string `json:"defaultWorkflow,omitempty"`
}

// SchemeResourceModel describes the workflow scheme resource data model.
type SchemeResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	DefaultWorkflowID types.String `tfsdk:"default_workflow_id"`
}

// SchemeResource implements the atlassian_jira_workflow_scheme managed resource.
type SchemeResource struct {
	client *atlassian.Client
}

// NewSchemeResource returns a new SchemeResource instance for provider registration.
func NewSchemeResource() resource.Resource {
	return &SchemeResource{}
}

// Metadata returns the resource type name.
func (r *SchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow_scheme"
}

// Schema defines the schema for the jira workflow scheme resource.
func (r *SchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira workflow scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the workflow scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the workflow scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the workflow scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_workflow_id": schema.StringAttribute{
				Description: "The ID of the default workflow for this scheme.",
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
func (r *SchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira workflow scheme.
func (r *SchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiWorkflowSchemeCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.DefaultWorkflowID.IsNull() && !plan.DefaultWorkflowID.IsUnknown() {
		body.DefaultWorkflowID = plan.DefaultWorkflowID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiWorkflowScheme
	err := r.client.Post(ctx, "/rest/api/3/workflowscheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate workflow scheme name",
					fmt.Sprintf("A workflow scheme with name %q already exists. Each workflow scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create workflow schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create workflow scheme",
			fmt.Sprintf("Could not create workflow scheme with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.DefaultWorkflowID = types.StringValue(created.DefaultWorkflowID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *SchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ws apiWorkflowScheme
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/workflowscheme/%s", state.ID.ValueString()), &ws)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read workflow scheme",
			fmt.Sprintf("Could not read workflow scheme %q: %s. Verify the workflow scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(ws.ID)
	state.Name = types.StringValue(ws.Name)
	state.Description = types.StringValue(ws.Description)
	state.DefaultWorkflowID = types.StringValue(ws.DefaultWorkflowID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira workflow scheme.
func (r *SchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiWorkflowSchemeUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.DefaultWorkflowID.IsNull() && !plan.DefaultWorkflowID.IsUnknown() {
		body.DefaultWorkflowID = plan.DefaultWorkflowID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiWorkflowScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/workflowscheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Workflow scheme not found",
					fmt.Sprintf("Workflow scheme with ID %q not found. The workflow scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update workflow schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update workflow scheme",
			fmt.Sprintf("Could not update workflow scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.DefaultWorkflowID = types.StringValue(updated.DefaultWorkflowID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira workflow scheme.
func (r *SchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/workflowscheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete workflow scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete workflow scheme",
			fmt.Sprintf("Could not delete workflow scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing workflow scheme by ID.
func (r *SchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
