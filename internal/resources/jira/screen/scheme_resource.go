// Package screen implements the atlassian_jira_screen_scheme managed resource.
//
// This resource manages Jira screen schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via screen scheme ID.
package screen

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

// apiScreenMapping represents a single operation-to-screen mapping in the API.
type apiScreenMapping struct {
	Operation string `json:"operation"`
	ScreenID  string `json:"screenId"`
}

// apiScreenScheme represents the JSON structure returned by the Atlassian screen scheme API.
type apiScreenScheme struct {
	ID          int                `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Screens     []apiScreenMapping `json:"screens,omitempty"`
}

// apiScreenSchemeCreate represents the JSON body for creating a screen scheme.
type apiScreenSchemeCreate struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	Screens     []apiScreenMapping `json:"screens,omitempty"`
}

// apiScreenSchemeUpdate represents the JSON body for updating a screen scheme.
type apiScreenSchemeUpdate struct {
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Screens     []apiScreenMapping `json:"screens,omitempty"`
}

// ScreenMappingModel describes a single operation-to-screen mapping in the Terraform model.
type ScreenMappingModel struct {
	Operation string `tfsdk:"operation"`
	ScreenID  string `tfsdk:"screen_id"`
}

// SchemeResourceModel describes the resource data model for screen schemes.
type SchemeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Screens     types.List   `tfsdk:"screens"`
}

// SchemeResource implements the atlassian_jira_screen_scheme managed resource.
type SchemeResource struct {
	client *atlassian.Client
}

// NewSchemeResource returns a new SchemeResource instance for provider registration.
func NewSchemeResource() resource.Resource {
	return &SchemeResource{}
}

// Metadata returns the resource type name.
func (r *SchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_scheme"
}

// Schema defines the schema for the jira screen scheme resource.
func (r *SchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira screen scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the screen scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the screen scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the screen scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"screens": schema.ListNestedAttribute{
				Description: "Mappings of operations to screen IDs.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"operation": schema.StringAttribute{
							Description: "The operation mapped to the screen (default, create, edit, or view).",
							Required:    true,
						},
						"screen_id": schema.StringAttribute{
							Description: "The ID of the screen assigned to this operation.",
							Required:    true,
						},
					},
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

// Create provisions a new Jira screen scheme.
func (r *SchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiScreenSchemeCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.Screens = screenMappingsFromPlan(ctx, plan.Screens)
	bodyBytes, _ := json.Marshal(body)

	var created apiScreenScheme
	err := r.client.Post(ctx, "/rest/api/3/screenscheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate screen scheme name",
					fmt.Sprintf("A screen scheme with name %q already exists. Each screen scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid screen scheme configuration",
					fmt.Sprintf("The screen scheme configuration for %q is invalid. Verify the screen scheme name and description are valid.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create screen schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create screen scheme",
			fmt.Sprintf("Could not create screen scheme %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", created.ID))
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.Screens = screenMappingsToState(ctx, created.Screens)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *SchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var scheme apiScreenScheme
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/screenscheme/%s", state.ID.ValueString()), &scheme)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read screen scheme",
			fmt.Sprintf("Could not read screen scheme %q: %s. Verify the screen scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(fmt.Sprintf("%d", scheme.ID))
	state.Name = types.StringValue(scheme.Name)
	state.Description = types.StringValue(scheme.Description)
	state.Screens = screenMappingsToState(ctx, scheme.Screens)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira screen scheme.
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

	body := apiScreenSchemeUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.Screens = screenMappingsFromPlan(ctx, plan.Screens)
	bodyBytes, _ := json.Marshal(body)

	var updated apiScreenScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/screenscheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Screen scheme not found",
					fmt.Sprintf("Screen scheme with ID %q not found. The screen scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid screen scheme configuration",
					fmt.Sprintf("The screen scheme update for ID %q is invalid. Verify the screen scheme name and description are valid.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update screen schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update screen scheme",
			fmt.Sprintf("Could not update screen scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", updated.ID))
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.Screens = screenMappingsToState(ctx, updated.Screens)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira screen scheme.
func (r *SchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/screenscheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete screen scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete screen scheme",
			fmt.Sprintf("Could not delete screen scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing screen scheme by ID.
func (r *SchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// screenMappingObjectType is the attr.Type for the screens nested object.
var screenMappingObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"operation": types.StringType,
		"screen_id": types.StringType,
	},
}

// screenMappingsFromPlan converts the Terraform screens list to API format.
func screenMappingsFromPlan(ctx context.Context, mappings types.List) []apiScreenMapping {
	if mappings.IsNull() || mappings.IsUnknown() {
		return nil
	}
	var models []ScreenMappingModel
	mappings.ElementsAs(ctx, &models, false)
	var result []apiScreenMapping
	for _, m := range models {
		result = append(result, apiScreenMapping{
			Operation: m.Operation,
			ScreenID:  m.ScreenID,
		})
	}
	return result
}

// screenMappingsToState converts API screen mappings to the Terraform state list.
func screenMappingsToState(_ context.Context, mappings []apiScreenMapping) types.List {
	if len(mappings) == 0 {
		return types.ListNull(screenMappingObjectType)
	}
	var elems []attr.Value
	for _, m := range mappings {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"operation": types.StringType,
				"screen_id": types.StringType,
			},
			map[string]attr.Value{
				"operation": types.StringValue(m.Operation),
				"screen_id": types.StringValue(m.ScreenID),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(screenMappingObjectType, elems)
	return list
}
