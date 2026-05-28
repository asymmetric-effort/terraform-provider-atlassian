// Package space implements the atlassian_confluence_space managed resource.
//
// This resource manages Confluence spaces through the Atlassian Cloud
// REST API (v2). It supports full CRUD operations and state import
// via space ID or key.
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

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiSpaceLinks is used to extract the webui link from the _links field.
type apiSpaceLinks struct {
	WebUI string `json:"webui"`
}

// apiSpaceDescription wraps the description object in the API response.
type apiSpaceDescription struct {
	Plain struct {
		Value string `json:"value"`
	} `json:"plain"`
}

// apiSpaceFullResponse represents the full API response with nested description.
type apiSpaceFullResponse struct {
	ID          string              `json:"id"`
	Key         string              `json:"key"`
	Name        string              `json:"name"`
	Description apiSpaceDescription `json:"description"`
	Type        string              `json:"type"`
	HomepageID  string              `json:"homepageId"`
	Status      string              `json:"status"`
	Links       apiSpaceLinks       `json:"_links"`
}

// apiSpaceCreate represents the JSON body for creating a Confluence space.
type apiSpaceCreate struct {
	Key         string              `json:"key"`
	Name        string              `json:"name"`
	Description *apiSpaceCreateDesc `json:"description,omitempty"`
	Type        string              `json:"type,omitempty"`
}

// apiSpaceCreateDesc wraps the description for the create request.
type apiSpaceCreateDesc struct {
	Plain struct {
		Value          string `json:"value"`
		Representation string `json:"representation"`
	} `json:"plain"`
}

// apiSpaceUpdate represents the JSON body for updating a Confluence space.
type apiSpaceUpdate struct {
	Name        string              `json:"name,omitempty"`
	Description *apiSpaceCreateDesc `json:"description,omitempty"`
	Type        string              `json:"type,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Key         types.String `tfsdk:"key"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	HomepageID  types.String `tfsdk:"homepage_id"`
	Status      types.String `tfsdk:"status"`
	URL         types.String `tfsdk:"url"`
}

// Resource implements the atlassian_confluence_space managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_space"
}

// Schema defines the schema for the Confluence space resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Confluence space in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the space, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Description: "The space key (e.g., ENG). Must be unique and cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the space.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A plain-text description of the space.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				Description: "The type of space. Must be \"global\" or \"personal\".",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"homepage_id": schema.StringAttribute{
				Description: "The ID of the space's homepage.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "The status of the space (e.g., current, archived).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				Description: "The URL of the space in Atlassian Cloud.",
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

// parseSpaceResponse parses a raw JSON response into ResourceModel fields.
func parseSpaceResponse(raw json.RawMessage) (id, key, name, description, spaceType, homepageID, status, url string) {
	var resp apiSpaceFullResponse
	if json.Unmarshal(raw, &resp) == nil {
		id = resp.ID
		key = resp.Key
		name = resp.Name
		description = resp.Description.Plain.Value
		spaceType = resp.Type
		homepageID = resp.HomepageID
		status = resp.Status
		url = resp.Links.WebUI
	}
	return
}

// Create provisions a new Confluence space.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiSpaceCreate{
		Key:  plan.Key.ValueString(),
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		desc := &apiSpaceCreateDesc{}
		desc.Plain.Value = plan.Description.ValueString()
		desc.Plain.Representation = "plain"
		body.Description = desc
	}
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() {
		body.Type = plan.Type.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var raw json.RawMessage
	err := r.client.Post(ctx, "/wiki/api/v2/spaces", bytes.NewReader(bodyBytes), &raw)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate space key",
					fmt.Sprintf("A Confluence space with key %q already exists. Each space key must be unique within the Atlassian organization.", plan.Key.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create Confluence spaces. Ensure the service account has Confluence admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create Confluence space",
			fmt.Sprintf("Could not create Confluence space with key %q: %s", plan.Key.ValueString(), err.Error()),
		)
		return
	}

	id, key, name, description, spaceType, homepageID, status, url := parseSpaceResponse(raw)
	plan.ID = types.StringValue(id)
	plan.Key = types.StringValue(key)
	plan.Name = types.StringValue(name)
	plan.Description = types.StringValue(description)
	plan.Type = types.StringValue(spaceType)
	plan.HomepageID = types.StringValue(homepageID)
	plan.Status = types.StringValue(status)
	plan.URL = types.StringValue(url)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := state.ID.ValueString()
	if identifier == "" {
		identifier = state.Key.ValueString()
	}

	var raw json.RawMessage
	err := r.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", identifier), &raw)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Confluence space",
			fmt.Sprintf("Could not read Confluence space %q: %s. Verify the space exists and has not been deleted.",
				identifier, err.Error()),
		)
		return
	}

	id, key, name, description, spaceType, homepageID, status, url := parseSpaceResponse(raw)
	state.ID = types.StringValue(id)
	state.Key = types.StringValue(key)
	state.Name = types.StringValue(name)
	state.Description = types.StringValue(description)
	state.Type = types.StringValue(spaceType)
	state.HomepageID = types.StringValue(homepageID)
	state.Status = types.StringValue(status)
	state.URL = types.StringValue(url)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Confluence space.
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

	body := apiSpaceUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		desc := &apiSpaceCreateDesc{}
		desc.Plain.Value = plan.Description.ValueString()
		desc.Plain.Representation = "plain"
		body.Description = desc
	}
	if !plan.Type.IsNull() && !plan.Type.IsUnknown() {
		body.Type = plan.Type.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var raw json.RawMessage
	err := r.client.Put(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &raw)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Confluence space not found",
					fmt.Sprintf("Confluence space with ID %q not found. The space may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update Confluence spaces. Ensure the service account has Confluence admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Confluence space",
			fmt.Sprintf("Could not update Confluence space with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	id, key, name, description, spaceType, homepageID, status, url := parseSpaceResponse(raw)
	plan.ID = types.StringValue(id)
	plan.Key = types.StringValue(key)
	plan.Name = types.StringValue(name)
	plan.Description = types.StringValue(description)
	plan.Type = types.StringValue(spaceType)
	plan.HomepageID = types.StringValue(homepageID)
	plan.Status = types.StringValue(status)
	plan.URL = types.StringValue(url)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Confluence space.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete Confluence space %q. "+
						"Ensure the service account has Confluence admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Confluence space",
			fmt.Sprintf("Could not delete Confluence space with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing Confluence space by ID or key.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
