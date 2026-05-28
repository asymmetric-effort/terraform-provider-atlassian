// Package board implements the atlassian_jira_board managed resource.
//
// This resource manages Jira boards through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via board ID.
package board

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

// apiColumnConfig represents a single column in the board column configuration.
type apiColumnConfig struct {
	Name      string   `json:"name"`
	StatusIDs []string `json:"statusIds,omitempty"`
}

// apiBoard represents the JSON structure returned by the Atlassian board API.
type apiBoard struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	SpaceID      string            `json:"spaceId"`
	Self         string            `json:"self"`
	ColumnConfig []apiColumnConfig `json:"columnConfig,omitempty"`
}

// apiBoardCreate represents the JSON body for creating a board.
type apiBoardCreate struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	SpaceID      string            `json:"spaceId"`
	ColumnConfig []apiColumnConfig `json:"columnConfig,omitempty"`
}

// apiBoardUpdate represents the JSON body for updating a board.
type apiBoardUpdate struct {
	Name         string            `json:"name,omitempty"`
	Type         string            `json:"type,omitempty"`
	SpaceID      string            `json:"spaceId,omitempty"`
	ColumnConfig []apiColumnConfig `json:"columnConfig,omitempty"`
}

// ColumnConfigModel describes a single column configuration entry in the Terraform model.
type ColumnConfigModel struct {
	Name      string   `tfsdk:"name"`
	StatusIDs []string `tfsdk:"status_ids"`
}

// columnConfigObjectType is the attr.Type for the column config nested object.
var columnConfigObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":       types.StringType,
		"status_ids": types.ListType{ElemType: types.StringType},
	},
}

// columnConfigFromPlan converts the Terraform column_config list to API format.
func columnConfigFromPlan(ctx context.Context, configs types.List) []apiColumnConfig {
	if configs.IsNull() || configs.IsUnknown() {
		return nil
	}
	var models []ColumnConfigModel
	configs.ElementsAs(ctx, &models, false)
	var result []apiColumnConfig
	for _, m := range models {
		result = append(result, apiColumnConfig{
			Name:      m.Name,
			StatusIDs: m.StatusIDs,
		})
	}
	return result
}

// columnConfigToState converts API column configs to the Terraform state list.
func columnConfigToState(ctx context.Context, configs []apiColumnConfig) types.List {
	if len(configs) == 0 {
		return types.ListNull(columnConfigObjectType)
	}
	var elems []attr.Value
	for _, c := range configs {
		var statusElems []attr.Value
		for _, s := range c.StatusIDs {
			statusElems = append(statusElems, types.StringValue(s))
		}
		var statusList types.List
		if len(statusElems) == 0 {
			statusList = types.ListNull(types.StringType)
		} else {
			statusList, _ = types.ListValue(types.StringType, statusElems)
		}
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"name":       types.StringType,
				"status_ids": types.ListType{ElemType: types.StringType},
			},
			map[string]attr.Value{
				"name":       types.StringValue(c.Name),
				"status_ids": statusList,
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(columnConfigObjectType, elems)
	_ = ctx
	return list
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Type         types.String `tfsdk:"type"`
	SpaceID      types.String `tfsdk:"space_id"`
	ColumnConfig types.List   `tfsdk:"column_config"`
}

// Resource implements the atlassian_jira_board managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_board"
}

// Schema defines the schema for the jira board resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira board in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the board, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the board.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of the board (scrum or kanban).",
				Required:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the space (project) associated with this board.",
				Required:    true,
			},
			"column_config": schema.ListNestedAttribute{
				Description: "Column configuration for the board, defining columns and their mapped statuses.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the column.",
							Required:    true,
						},
						"status_ids": schema.ListAttribute{
							Description: "List of status IDs mapped to this column.",
							Optional:    true,
							ElementType: types.StringType,
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

// Create provisions a new Jira board.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiBoardCreate{
		Name:         plan.Name.ValueString(),
		Type:         plan.Type.ValueString(),
		SpaceID:      plan.SpaceID.ValueString(),
		ColumnConfig: columnConfigFromPlan(ctx, plan.ColumnConfig),
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiBoard
	err := r.client.Post(ctx, "/rest/agile/1.0/board", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate board name",
					fmt.Sprintf("A board with name %q already exists. Each board name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid board configuration",
					fmt.Sprintf("The board configuration for %q is invalid. Verify that the board type is \"scrum\" or \"kanban\" and the space_id references an existing space.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create boards. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create board",
			fmt.Sprintf("Could not create board with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Type = types.StringValue(created.Type)
	plan.SpaceID = types.StringValue(created.SpaceID)
	plan.ColumnConfig = columnConfigToState(ctx, created.ColumnConfig)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var b apiBoard
	err := r.client.Get(ctx, fmt.Sprintf("/rest/agile/1.0/board/%s", state.ID.ValueString()), &b)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read board",
			fmt.Sprintf("Could not read board %q: %s. Verify the board exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(b.ID)
	state.Name = types.StringValue(b.Name)
	state.Type = types.StringValue(b.Type)
	state.SpaceID = types.StringValue(b.SpaceID)
	state.ColumnConfig = columnConfigToState(ctx, b.ColumnConfig)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira board.
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

	body := apiBoardUpdate{
		Name:         plan.Name.ValueString(),
		Type:         plan.Type.ValueString(),
		SpaceID:      plan.SpaceID.ValueString(),
		ColumnConfig: columnConfigFromPlan(ctx, plan.ColumnConfig),
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiBoard
	err := r.client.Put(ctx, fmt.Sprintf("/rest/agile/1.0/board/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Board not found",
					fmt.Sprintf("Board with ID %q not found. The board may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid board configuration",
					fmt.Sprintf("The board update for ID %q is invalid. Verify that the board type is \"scrum\" or \"kanban\" and the space_id references an existing space.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update boards. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update board",
			fmt.Sprintf("Could not update board with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Type = types.StringValue(updated.Type)
	plan.SpaceID = types.StringValue(updated.SpaceID)
	plan.ColumnConfig = columnConfigToState(ctx, updated.ColumnConfig)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira board.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/agile/1.0/board/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete board %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete board",
			fmt.Sprintf("Could not delete board with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing board by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
