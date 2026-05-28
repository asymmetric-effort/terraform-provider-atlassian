// Package priority implements the atlassian_jira_priority_scheme managed resource.
//
// This resource manages Jira priority schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via priority scheme ID.
package priority

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

// Ensure the SchemeResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &SchemeResource{}
	_ resource.ResourceWithImportState = &SchemeResource{}
)

// apiPriorityScheme represents the JSON structure returned by the Atlassian priority scheme API.
type apiPriorityScheme struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	PriorityIDs       []string `json:"priorityIds,omitempty"`
	DefaultPriorityID string   `json:"defaultPriorityId,omitempty"`
	Self              string   `json:"self"`
}

// apiPrioritySchemeCreate represents the JSON body for creating a priority scheme.
type apiPrioritySchemeCreate struct {
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	PriorityIDs       []string `json:"priorityIds,omitempty"`
	DefaultPriorityID string   `json:"defaultPriorityId,omitempty"`
}

// apiPrioritySchemeUpdate represents the JSON body for updating a priority scheme.
type apiPrioritySchemeUpdate struct {
	Name              string   `json:"name,omitempty"`
	Description       string   `json:"description,omitempty"`
	PriorityIDs       []string `json:"priorityIds,omitempty"`
	DefaultPriorityID string   `json:"defaultPriorityId,omitempty"`
}

// SchemeResourceModel describes the priority scheme resource data model.
type SchemeResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	PriorityIDs       types.List   `tfsdk:"priority_ids"`
	DefaultPriorityID types.String `tfsdk:"default_priority_id"`
}

// SchemeResource implements the atlassian_jira_priority_scheme managed resource.
type SchemeResource struct {
	client *atlassian.Client
}

// NewSchemeResource returns a new SchemeResource instance for provider registration.
func NewSchemeResource() resource.Resource {
	return &SchemeResource{}
}

// Metadata returns the resource type name.
func (r *SchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_priority_scheme"
}

// Schema defines the schema for the jira priority scheme resource.
func (r *SchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira priority scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the priority scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the priority scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the priority scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"priority_ids": schema.ListAttribute{
				Description: "Ordered list of priority IDs in the scheme, defining priority ordering.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"default_priority_id": schema.StringAttribute{
				Description: "The default priority ID for this scheme.",
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

// extractPriorityIDs converts the types.List to a Go string slice.
// The schema guarantees all elements are types.String, so the type assertion is safe.
func extractPriorityIDs(list types.List) ([]string, bool) {
	if list.IsNull() || list.IsUnknown() {
		return nil, false
	}
	var ids []string
	for _, elem := range list.Elements() {
		// Safe assertion: schema enforces element type as types.StringType.
		ids = append(ids, elem.(types.String).ValueString())
	}
	return ids, true
}

// buildPriorityIDsList converts a string slice to a types.List of StringType values.
func buildPriorityIDsList(ids []string) types.List {
	if len(ids) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, len(ids))
	for i, id := range ids {
		elems[i] = types.StringValue(id)
	}
	return types.ListValueMust(types.StringType, elems)
}

// Create provisions a new Jira priority scheme.
func (r *SchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiPrioritySchemeCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if ids, ok := extractPriorityIDs(plan.PriorityIDs); ok {
		body.PriorityIDs = ids
	}
	if !plan.DefaultPriorityID.IsNull() && !plan.DefaultPriorityID.IsUnknown() {
		body.DefaultPriorityID = plan.DefaultPriorityID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiPriorityScheme
	err := r.client.Post(ctx, "/rest/api/3/priorityscheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate priority scheme name",
					fmt.Sprintf("A priority scheme with name %q already exists. Each priority scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid priority scheme configuration",
					fmt.Sprintf("The priority scheme configuration for %q is invalid. Verify the scheme name and priority mappings are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create priority schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create priority scheme",
			fmt.Sprintf("Could not create priority scheme with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.PriorityIDs = buildPriorityIDsList(created.PriorityIDs)
	plan.DefaultPriorityID = types.StringValue(created.DefaultPriorityID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *SchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ps apiPriorityScheme
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/priorityscheme/%s", state.ID.ValueString()), &ps)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read priority scheme",
			fmt.Sprintf("Could not read priority scheme %q: %s. Verify the priority scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(ps.ID)
	state.Name = types.StringValue(ps.Name)
	state.Description = types.StringValue(ps.Description)
	state.PriorityIDs = buildPriorityIDsList(ps.PriorityIDs)
	state.DefaultPriorityID = types.StringValue(ps.DefaultPriorityID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira priority scheme.
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

	body := apiPrioritySchemeUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if ids, ok := extractPriorityIDs(plan.PriorityIDs); ok {
		body.PriorityIDs = ids
	}
	if !plan.DefaultPriorityID.IsNull() && !plan.DefaultPriorityID.IsUnknown() {
		body.DefaultPriorityID = plan.DefaultPriorityID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiPriorityScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/priorityscheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Priority scheme not found",
					fmt.Sprintf("Priority scheme with ID %q not found. The priority scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid priority scheme configuration",
					fmt.Sprintf("The priority scheme update for ID %q is invalid. Verify the scheme name and priority mappings are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update priority schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update priority scheme",
			fmt.Sprintf("Could not update priority scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.PriorityIDs = buildPriorityIDsList(updated.PriorityIDs)
	plan.DefaultPriorityID = types.StringValue(updated.DefaultPriorityID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira priority scheme.
func (r *SchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/priorityscheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete priority scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete priority scheme",
			fmt.Sprintf("Could not delete priority scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing priority scheme by ID.
func (r *SchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
