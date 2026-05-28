// Package component implements the atlassian_statuspage_component managed resource.
//
// This resource manages Statuspage components through the Atlassian
// Statuspage REST API (v1). It supports full CRUD operations and
// state import via component ID.
package component

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

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiComponent represents the JSON structure returned by the Statuspage component API.
type apiComponent struct {
	ID          string `json:"id"`
	PageID      string `json:"page_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	GroupID     string `json:"group_id"`
}

// apiComponentCreate represents the JSON body for creating a component.
type apiComponentCreate struct {
	Component apiComponentBody `json:"component"`
}

// apiComponentBody holds the component fields.
type apiComponentBody struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID          types.String `tfsdk:"id"`
	PageID      types.String `tfsdk:"page_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	GroupID     types.String `tfsdk:"group_id"`
}

// Resource implements the atlassian_statuspage_component managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_component"
}

// Schema defines the schema for the component resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Statuspage component.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the component.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page this component belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the component.",
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
			"status": schema.StringAttribute{
				Description: "The status of the component (e.g., operational, degraded_performance, partial_outage, major_outage).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"group_id": schema.StringAttribute{
				Description: "The ID of the component group this component belongs to.",
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

// Create provisions a new component.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiComponentCreate{
		Component: apiComponentBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Component.Description = plan.Description.ValueString()
	}
	if !plan.Status.IsNull() && !plan.Status.IsUnknown() {
		body.Component.Status = plan.Status.ValueString()
	}
	if !plan.GroupID.IsNull() && !plan.GroupID.IsUnknown() {
		body.Component.GroupID = plan.GroupID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiComponent
	err := r.client.Post(ctx, fmt.Sprintf("/v1/pages/%s/components", plan.PageID.ValueString()), bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusForbidden {
			resp.Diagnostics.AddError(
				"Permission denied",
				"The authenticated user does not have permission to create Statuspage components. Ensure the service account has Statuspage admin privileges.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to create Statuspage component",
			fmt.Sprintf("Could not create component with name %q on page %q: %s",
				plan.Name.ValueString(), plan.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.PageID = types.StringValue(created.PageID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.Status = types.StringValue(created.Status)
	plan.GroupID = types.StringValue(created.GroupID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var comp apiComponent
	err := r.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/components/%s", state.PageID.ValueString(), state.ID.ValueString()), &comp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage component",
			fmt.Sprintf("Could not read component %q on page %q: %s. Verify the component exists and has not been deleted.",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(comp.ID)
	state.PageID = types.StringValue(comp.PageID)
	state.Name = types.StringValue(comp.Name)
	state.Description = types.StringValue(comp.Description)
	state.Status = types.StringValue(comp.Status)
	state.GroupID = types.StringValue(comp.GroupID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing component.
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

	body := apiComponentCreate{
		Component: apiComponentBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Component.Description = plan.Description.ValueString()
	}
	if !plan.Status.IsNull() && !plan.Status.IsUnknown() {
		body.Component.Status = plan.Status.ValueString()
	}
	if !plan.GroupID.IsNull() && !plan.GroupID.IsUnknown() {
		body.Component.GroupID = plan.GroupID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiComponent
	err := r.client.Put(ctx, fmt.Sprintf("/v1/pages/%s/components/%s", state.PageID.ValueString(), state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Component not found",
					fmt.Sprintf("Component %q on page %q not found. The component may have been deleted outside of Terraform.",
						state.ID.ValueString(), state.PageID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update Statuspage components. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Statuspage component",
			fmt.Sprintf("Could not update component %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.PageID = types.StringValue(updated.PageID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.Status = types.StringValue(updated.Status)
	plan.GroupID = types.StringValue(updated.GroupID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a component.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/v1/pages/%s/components/%s", state.PageID.ValueString(), state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete component %q. "+
						"Ensure the service account has Statuspage admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Statuspage component",
			fmt.Sprintf("Could not delete component %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing component by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
