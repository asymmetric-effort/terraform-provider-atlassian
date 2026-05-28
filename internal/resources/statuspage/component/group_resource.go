// Package component implements the atlassian_statuspage_component_group managed resource.
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

// Ensure the GroupResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &GroupResource{}
	_ resource.ResourceWithImportState = &GroupResource{}
)

// apiComponentGroup represents the JSON structure returned by the component group API.
type apiComponentGroup struct {
	ID          string `json:"id"`
	PageID      string `json:"page_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// apiComponentGroupCreate represents the JSON body for creating a component group.
type apiComponentGroupCreate struct {
	ComponentGroup apiComponentGroupBody `json:"component_group"`
}

// apiComponentGroupBody holds the group fields.
type apiComponentGroupBody struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GroupResourceModel describes the resource data model.
type GroupResourceModel struct {
	ID          types.String `tfsdk:"id"`
	PageID      types.String `tfsdk:"page_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// GroupResource implements the atlassian_statuspage_component_group managed resource.
type GroupResource struct {
	client *atlassian.Client
}

// NewGroupResource returns a new GroupResource instance for provider registration.
func NewGroupResource() resource.Resource {
	return &GroupResource{}
}

// Metadata returns the resource type name.
func (r *GroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_component_group"
}

// Schema defines the schema for the component group resource.
func (r *GroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Statuspage component group.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the component group.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page this group belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the component group.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the component group.",
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
func (r *GroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new component group.
func (r *GroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiComponentGroupCreate{
		ComponentGroup: apiComponentGroupBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.ComponentGroup.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiComponentGroup
	err := r.client.Post(ctx, fmt.Sprintf("/v1/pages/%s/component-groups", plan.PageID.ValueString()), bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusForbidden {
			resp.Diagnostics.AddError(
				"Permission denied",
				"The authenticated user does not have permission to create component groups. Ensure the service account has Statuspage admin privileges.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to create Statuspage component group",
			fmt.Sprintf("Could not create component group with name %q on page %q: %s",
				plan.Name.ValueString(), plan.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.PageID = types.StringValue(created.PageID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *GroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var group apiComponentGroup
	err := r.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/component-groups/%s", state.PageID.ValueString(), state.ID.ValueString()), &group)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage component group",
			fmt.Sprintf("Could not read component group %q on page %q: %s. Verify the group exists and has not been deleted.",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(group.ID)
	state.PageID = types.StringValue(group.PageID)
	state.Name = types.StringValue(group.Name)
	state.Description = types.StringValue(group.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing component group.
func (r *GroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan GroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiComponentGroupCreate{
		ComponentGroup: apiComponentGroupBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.ComponentGroup.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiComponentGroup
	err := r.client.Put(ctx, fmt.Sprintf("/v1/pages/%s/component-groups/%s", state.PageID.ValueString(), state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Component group not found",
					fmt.Sprintf("Component group %q on page %q not found. The group may have been deleted outside of Terraform.",
						state.ID.ValueString(), state.PageID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update component groups. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Statuspage component group",
			fmt.Sprintf("Could not update component group %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.PageID = types.StringValue(updated.PageID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a component group.
func (r *GroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/v1/pages/%s/component-groups/%s", state.PageID.ValueString(), state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete component group %q. "+
						"Ensure the service account has Statuspage admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Statuspage component group",
			fmt.Sprintf("Could not delete component group %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing component group by ID.
func (r *GroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
