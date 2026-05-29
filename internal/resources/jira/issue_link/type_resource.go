// Package issuelink implements the atlassian_jira_issue_link_type managed resource.
//
// This resource manages Jira issue link types through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via issue link type ID.
package issuelink

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

// Ensure the TypeResource satisfies framework interfaces.
var (
	_ resource.Resource                = &TypeResource{}
	_ resource.ResourceWithImportState = &TypeResource{}
)

// apiIssueLinkType represents the JSON structure returned by the Atlassian issue link type API.
type apiIssueLinkType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
	Self    string `json:"self"`
}

// apiIssueLinkTypeCreate represents the JSON body for creating an issue link type.
type apiIssueLinkTypeCreate struct {
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// TypeResourceModel describes the issue link type resource data model.
type TypeResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Inward  types.String `tfsdk:"inward"`
	Outward types.String `tfsdk:"outward"`
}

// TypeResource implements the atlassian_jira_issue_link_type managed resource.
type TypeResource struct {
	client *atlassian.Client
}

// NewTypeResource returns a new TypeResource instance for provider registration.
func NewTypeResource() resource.Resource {
	return &TypeResource{}
}

// Metadata returns the resource type name.
func (r *TypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_link_type"
}

// Schema defines the schema for the jira issue link type resource.
func (r *TypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira issue link type in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the issue link type, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the issue link type (e.g., 'Blocks').",
				Required:    true,
			},
			"inward": schema.StringAttribute{
				Description: "The inward description of the link (e.g., 'is blocked by').",
				Required:    true,
			},
			"outward": schema.StringAttribute{
				Description: "The outward description of the link (e.g., 'blocks').",
				Required:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *TypeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira issue link type.
func (r *TypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIssueLinkTypeCreate{
		Name:    plan.Name.ValueString(),
		Inward:  plan.Inward.ValueString(),
		Outward: plan.Outward.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiIssueLinkType
	err := r.client.Post(ctx, "/rest/api/3/issueLinkType", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate issue link type name",
					fmt.Sprintf("An issue link type with name %q already exists. Each issue link type name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid issue link type",
					fmt.Sprintf("The issue link type %q is invalid. Verify the name, inward, and outward fields are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create issue link types. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create issue link type",
			fmt.Sprintf("Could not create issue link type with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Inward = types.StringValue(created.Inward)
	plan.Outward = types.StringValue(created.Outward)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *TypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ilt apiIssueLinkType
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/issueLinkType/%s", state.ID.ValueString()), &ilt)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read issue link type",
			fmt.Sprintf("Could not read issue link type %q: %s. Verify the issue link type exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(ilt.ID)
	state.Name = types.StringValue(ilt.Name)
	state.Inward = types.StringValue(ilt.Inward)
	state.Outward = types.StringValue(ilt.Outward)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira issue link type.
func (r *TypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state TypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIssueLinkType{
		Name:    plan.Name.ValueString(),
		Inward:  plan.Inward.ValueString(),
		Outward: plan.Outward.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiIssueLinkType
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/issueLinkType/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Issue link type not found",
					fmt.Sprintf("Issue link type with ID %q not found. The issue link type may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid issue link type",
					fmt.Sprintf("The issue link type update for ID %q is invalid. Verify the name, inward, and outward fields are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update issue link types. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update issue link type",
			fmt.Sprintf("Could not update issue link type with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Inward = types.StringValue(updated.Inward)
	plan.Outward = types.StringValue(updated.Outward)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira issue link type.
func (r *TypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/issueLinkType/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete issue link type %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete issue link type",
			fmt.Sprintf("Could not delete issue link type with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing issue link type by ID.
func (r *TypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
