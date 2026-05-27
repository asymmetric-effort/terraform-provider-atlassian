// Package group implements the atlassian_group managed resource.
//
// This resource manages Atlassian Cloud groups via the REST API.
// It supports full CRUD operations and ImportState for tofu import.
package group

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure Resource satisfies required interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// Resource implements the atlassian_group managed resource.
type Resource struct {
	client *atlassian.Client
}

// ResourceModel describes the resource data model for an Atlassian group.
type ResourceModel struct {
	GroupID types.String `tfsdk:"group_id"`
	Name    types.String `tfsdk:"name"`
	SelfURL types.String `tfsdk:"self_url"`
}

// apiGroupResponse represents the JSON response from the Atlassian group API.
type apiGroupResponse struct {
	GroupID string `json:"groupId"`
	Name    string `json:"name"`
	Self    string `json:"self"`
}

// apiGroupCreateRequest represents the JSON body for creating a group.
type apiGroupCreateRequest struct {
	Name string `json:"name"`
}

// NewResource returns a new instance of the group resource.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

// Schema defines the schema for the group resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Atlassian Cloud group.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				Description: "The unique identifier of the group, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the group. Must be unique within the Atlassian organization.",
				Required:    true,
			},
			"self_url": schema.StringAttribute{
				Description: "The URL of the group in the Atlassian REST API.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure retrieves the provider-configured client for API calls.
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

// Create creates a new Atlassian group.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	reqBody := apiGroupCreateRequest{
		Name: plan.Name.ValueString(),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to marshal group create request",
			err.Error(),
		)
		return
	}

	var apiResp apiGroupResponse
	err = r.client.Post(ctx, "/rest/api/3/group", bytes.NewReader(bodyBytes), &apiResp)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to create group",
			"Could not create group '"+plan.Name.ValueString()+"': "+err.Error(),
		)
		return
	}

	plan.GroupID = types.StringValue(apiResp.GroupID)
	plan.Name = types.StringValue(apiResp.Name)
	plan.SelfURL = types.StringValue(apiResp.Self)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read retrieves the current state of an Atlassian group.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiResp apiGroupResponse
	groupID := state.GroupID.ValueString()
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read group",
			"Could not read group (ID: "+groupID+"): "+err.Error(),
		)
		return
	}

	state.GroupID = types.StringValue(apiResp.GroupID)
	state.Name = types.StringValue(apiResp.Name)
	state.SelfURL = types.StringValue(apiResp.Self)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Atlassian group (name change).
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

	// The Atlassian group API does not support renaming groups directly.
	// We must delete the old group and create a new one.
	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", state.GroupID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 403 {
			resp.Diagnostics.AddError(
				"Permission denied",
				"The authenticated user does not have permission to update group '"+state.Name.ValueString()+"'. "+
					"Ensure the user has the 'Browse users and groups' global permission.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to update group",
			"Could not delete old group '"+state.Name.ValueString()+"' during update: "+err.Error(),
		)
		return
	}

	reqBody := apiGroupCreateRequest{
		Name: plan.Name.ValueString(),
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to marshal group create request",
			err.Error(),
		)
		return
	}

	var apiResp apiGroupResponse
	err = r.client.Post(ctx, "/rest/api/3/group", bytes.NewReader(bodyBytes), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 409 {
			resp.Diagnostics.AddError(
				"Duplicate group name",
				"A group with the name '"+plan.Name.ValueString()+"' already exists.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to update group",
			"Could not create replacement group '"+plan.Name.ValueString()+"' during update: "+err.Error(),
		)
		return
	}

	plan.GroupID = types.StringValue(apiResp.GroupID)
	plan.Name = types.StringValue(apiResp.Name)
	plan.SelfURL = types.StringValue(apiResp.Self)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an Atlassian group.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := state.GroupID.ValueString()
	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 404:
				// Group already deleted; nothing to do.
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to delete group '"+state.Name.ValueString()+"'. "+
						"Ensure the user has the 'Browse users and groups' global permission.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete group",
			"Could not delete group '"+state.Name.ValueString()+"' (ID: "+groupID+"): "+err.Error(),
		)
	}
}

// ImportState imports an existing Atlassian group by its group ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("group_id"), req, resp)
}
