// Package dashboard implements the atlassian_jira_filter managed resource.
//
// This resource manages Jira filters through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via filter ID.
package dashboard

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

// Ensure the FilterResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &FilterResource{}
	_ resource.ResourceWithImportState = &FilterResource{}
)

// apiFilter represents the JSON structure returned by the Atlassian filter API.
type apiFilter struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	JQL         string `json:"jql"`
	Self        string `json:"self"`
}

// apiFilterCreate represents the JSON body for creating a filter.
type apiFilterCreate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	JQL         string `json:"jql"`
}

// apiFilterUpdate represents the JSON body for updating a filter.
type apiFilterUpdate struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	JQL         string `json:"jql,omitempty"`
}

// FilterResourceModel describes the filter resource data model.
type FilterResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	JQL         types.String `tfsdk:"jql"`
}

// FilterResource implements the atlassian_jira_filter managed resource.
type FilterResource struct {
	client *atlassian.Client
}

// NewFilterResource returns a new FilterResource instance for provider registration.
func NewFilterResource() resource.Resource {
	return &FilterResource{}
}

// Metadata returns the resource type name.
func (r *FilterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_filter"
}

// Schema defines the schema for the jira filter resource.
func (r *FilterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira filter in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the filter, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the filter.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the filter.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"jql": schema.StringAttribute{
				Description: "The JQL query for the filter.",
				Required:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *FilterResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira filter.
func (r *FilterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiFilterCreate{
		Name: plan.Name.ValueString(),
		JQL:  plan.JQL.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiFilter
	err := r.client.Post(ctx, "/rest/api/3/filter", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate filter name",
					fmt.Sprintf("A filter with name %q already exists. Each filter name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create filters. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create filter",
			fmt.Sprintf("Could not create filter with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.JQL = types.StringValue(created.JQL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *FilterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var f apiFilter
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/filter/%s", state.ID.ValueString()), &f)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read filter",
			fmt.Sprintf("Could not read filter %q: %s. Verify the filter exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(f.ID)
	state.Name = types.StringValue(f.Name)
	state.Description = types.StringValue(f.Description)
	state.JQL = types.StringValue(f.JQL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira filter.
func (r *FilterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state FilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiFilterUpdate{
		Name: plan.Name.ValueString(),
		JQL:  plan.JQL.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiFilter
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/filter/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Filter not found",
					fmt.Sprintf("Filter with ID %q not found. The filter may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update filters. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update filter",
			fmt.Sprintf("Could not update filter with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.JQL = types.StringValue(updated.JQL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira filter.
func (r *FilterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/filter/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete filter %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete filter",
			fmt.Sprintf("Could not delete filter with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing filter by ID.
func (r *FilterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
