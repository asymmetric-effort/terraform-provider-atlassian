// Package screen implements the atlassian_jira_issue_type_screen_scheme managed resource.
//
// This resource manages Jira issue type screen schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via issue type screen scheme ID.
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

// Ensure the IssueTypeScreenSchemeResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &IssueTypeScreenSchemeResource{}
	_ resource.ResourceWithImportState = &IssueTypeScreenSchemeResource{}
)

// apiIssueTypeScreenMapping represents a single issue-type-to-screen-scheme mapping in the API.
type apiIssueTypeScreenMapping struct {
	IssueTypeID    string `json:"issueTypeId"`
	ScreenSchemeID string `json:"screenSchemeId"`
}

// apiIssueTypeScreenScheme represents the JSON structure returned by the Atlassian issue type screen scheme API.
type apiIssueTypeScreenScheme struct {
	ID                string                      `json:"id"`
	Name              string                      `json:"name"`
	Description       string                      `json:"description"`
	IssueTypeMappings []apiIssueTypeScreenMapping `json:"issueTypeMappings,omitempty"`
}

// apiIssueTypeScreenSchemeCreate represents the JSON body for creating an issue type screen scheme.
type apiIssueTypeScreenSchemeCreate struct {
	Name              string                      `json:"name"`
	Description       string                      `json:"description,omitempty"`
	IssueTypeMappings []apiIssueTypeScreenMapping `json:"issueTypeMappings,omitempty"`
}

// apiIssueTypeScreenSchemeUpdate represents the JSON body for updating an issue type screen scheme.
type apiIssueTypeScreenSchemeUpdate struct {
	Name              string                      `json:"name,omitempty"`
	Description       string                      `json:"description,omitempty"`
	IssueTypeMappings []apiIssueTypeScreenMapping `json:"issueTypeMappings,omitempty"`
}

// IssueTypeScreenMappingModel describes a single issue-type-to-screen-scheme mapping in the Terraform model.
type IssueTypeScreenMappingModel struct {
	IssueTypeID    string `tfsdk:"issue_type_id"`
	ScreenSchemeID string `tfsdk:"screen_scheme_id"`
}

// IssueTypeScreenSchemeResourceModel describes the resource data model for issue type screen schemes.
type IssueTypeScreenSchemeResourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	IssueTypeMappings types.List   `tfsdk:"issue_type_mappings"`
}

// IssueTypeScreenSchemeResource implements the atlassian_jira_issue_type_screen_scheme managed resource.
type IssueTypeScreenSchemeResource struct {
	client *atlassian.Client
}

// NewIssueTypeScreenSchemeResource returns a new IssueTypeScreenSchemeResource instance for provider registration.
func NewIssueTypeScreenSchemeResource() resource.Resource {
	return &IssueTypeScreenSchemeResource{}
}

// Metadata returns the resource type name.
func (r *IssueTypeScreenSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type_screen_scheme"
}

// Schema defines the schema for the jira issue type screen scheme resource.
func (r *IssueTypeScreenSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira issue type screen scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the issue type screen scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the issue type screen scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the issue type screen scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"issue_type_mappings": schema.ListNestedAttribute{
				Description: "Mappings of issue types to screen schemes.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"issue_type_id": schema.StringAttribute{
							Description: "The ID of the issue type.",
							Required:    true,
						},
						"screen_scheme_id": schema.StringAttribute{
							Description: "The ID of the screen scheme assigned to this issue type.",
							Required:    true,
						},
					},
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *IssueTypeScreenSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira issue type screen scheme.
func (r *IssueTypeScreenSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIssueTypeScreenSchemeCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.IssueTypeMappings = issueTypeScreenMappingsFromPlan(ctx, plan.IssueTypeMappings)
	bodyBytes, _ := json.Marshal(body)

	var created apiIssueTypeScreenScheme
	err := r.client.Post(ctx, "/rest/api/3/issuetypescreenscheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate issue type screen scheme name",
					fmt.Sprintf("An issue type screen scheme with name %q already exists. Each issue type screen scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid issue type screen scheme configuration",
					fmt.Sprintf("The issue type screen scheme configuration for %q is invalid. Verify the issue type mappings are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create issue type screen schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create issue type screen scheme",
			fmt.Sprintf("Could not create issue type screen scheme %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.IssueTypeMappings = issueTypeScreenMappingsToState(ctx, created.IssueTypeMappings)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *IssueTypeScreenSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var scheme apiIssueTypeScreenScheme
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s", state.ID.ValueString()), &scheme)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read issue type screen scheme",
			fmt.Sprintf("Could not read issue type screen scheme %q: %s. Verify the issue type screen scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(scheme.ID)
	state.Name = types.StringValue(scheme.Name)
	state.Description = types.StringValue(scheme.Description)
	state.IssueTypeMappings = issueTypeScreenMappingsToState(ctx, scheme.IssueTypeMappings)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira issue type screen scheme.
func (r *IssueTypeScreenSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state IssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIssueTypeScreenSchemeUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.IssueTypeMappings = issueTypeScreenMappingsFromPlan(ctx, plan.IssueTypeMappings)
	bodyBytes, _ := json.Marshal(body)

	var updated apiIssueTypeScreenScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Issue type screen scheme not found",
					fmt.Sprintf("Issue type screen scheme with ID %q not found. The issue type screen scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid issue type screen scheme configuration",
					fmt.Sprintf("The issue type screen scheme update for ID %q is invalid. Verify the issue type mappings are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update issue type screen schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update issue type screen scheme",
			fmt.Sprintf("Could not update issue type screen scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.IssueTypeMappings = issueTypeScreenMappingsToState(ctx, updated.IssueTypeMappings)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira issue type screen scheme.
func (r *IssueTypeScreenSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IssueTypeScreenSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete issue type screen scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete issue type screen scheme",
			fmt.Sprintf("Could not delete issue type screen scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing issue type screen scheme by ID.
func (r *IssueTypeScreenSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// issueTypeScreenMappingObjectType is the attr.Type for the issue_type_mappings nested object.
var issueTypeScreenMappingObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"issue_type_id":    types.StringType,
		"screen_scheme_id": types.StringType,
	},
}

// issueTypeScreenMappingsFromPlan converts the Terraform issue_type_mappings list to API format.
func issueTypeScreenMappingsFromPlan(ctx context.Context, mappings types.List) []apiIssueTypeScreenMapping {
	if mappings.IsNull() || mappings.IsUnknown() {
		return nil
	}
	var models []IssueTypeScreenMappingModel
	mappings.ElementsAs(ctx, &models, false)
	var result []apiIssueTypeScreenMapping
	for _, m := range models {
		result = append(result, apiIssueTypeScreenMapping{
			IssueTypeID:    m.IssueTypeID,
			ScreenSchemeID: m.ScreenSchemeID,
		})
	}
	return result
}

// issueTypeScreenMappingsToState converts API issue type screen mappings to the Terraform state list.
func issueTypeScreenMappingsToState(_ context.Context, mappings []apiIssueTypeScreenMapping) types.List {
	if len(mappings) == 0 {
		return types.ListNull(issueTypeScreenMappingObjectType)
	}
	var elems []attr.Value
	for _, m := range mappings {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"issue_type_id":    types.StringType,
				"screen_scheme_id": types.StringType,
			},
			map[string]attr.Value{
				"issue_type_id":    types.StringValue(m.IssueTypeID),
				"screen_scheme_id": types.StringValue(m.ScreenSchemeID),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(issueTypeScreenMappingObjectType, elems)
	return list
}
