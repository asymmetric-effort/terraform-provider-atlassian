// Package issuetype implements the atlassian_jira_issue_type_scheme managed resource.
//
// This resource manages Jira issue type schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via issue type scheme ID.
package issuetype

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

// Ensure the SchemeResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &SchemeResource{}
	_ resource.ResourceWithImportState = &SchemeResource{}
)

// apiIssueTypeScheme represents the JSON structure returned by the Atlassian issue type scheme API.
type apiIssueTypeScheme struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	DefaultIssueTypeID string `json:"defaultIssueTypeId,omitempty"`
	Self               string `json:"self"`
}

// apiIssueTypeSchemeCreate represents the JSON body for creating an issue type scheme.
type apiIssueTypeSchemeCreate struct {
	Name               string   `json:"name"`
	Description        string   `json:"description,omitempty"`
	IssueTypeIDs       []string `json:"issueTypeIds"`
	DefaultIssueTypeID string   `json:"defaultIssueTypeId,omitempty"`
}

// apiIssueTypeSchemeUpdate represents the JSON body for updating an issue type scheme.
type apiIssueTypeSchemeUpdate struct {
	Name               string   `json:"name,omitempty"`
	Description        string   `json:"description,omitempty"`
	IssueTypeIDs       []string `json:"issueTypeIds,omitempty"`
	DefaultIssueTypeID string   `json:"defaultIssueTypeId,omitempty"`
}

// SchemeResourceModel describes the scheme resource data model.
type SchemeResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	IssueTypeIDs       types.List   `tfsdk:"issue_type_ids"`
	DefaultIssueTypeID types.String `tfsdk:"default_issue_type_id"`
}

// SchemeResource implements the atlassian_jira_issue_type_scheme managed resource.
type SchemeResource struct {
	client *atlassian.Client
}

// NewSchemeResource returns a new SchemeResource instance for provider registration.
func NewSchemeResource() resource.Resource {
	return &SchemeResource{}
}

// Metadata returns the resource type name.
func (r *SchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type_scheme"
}

// Schema defines the schema for the jira issue type scheme resource.
func (r *SchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira issue type scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the issue type scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the issue type scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the issue type scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"issue_type_ids": schema.ListAttribute{
				Description: "The list of issue type IDs included in this scheme.",
				Required:    true,
				ElementType: types.StringType,
			},
			"default_issue_type_id": schema.StringAttribute{
				Description: "The default issue type ID for this scheme.",
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

// extractIssueTypeIDs converts the types.List to a Go string slice.
func extractIssueTypeIDs(ctx context.Context, list types.List) ([]string, bool) {
	if list.IsNull() || list.IsUnknown() {
		return nil, false
	}
	var ids []string
	for _, elem := range list.Elements() {
		strVal, ok := elem.(types.String)
		if !ok {
			return nil, false
		}
		ids = append(ids, strVal.ValueString())
	}
	return ids, true
}

// Create provisions a new Jira issue type scheme.
func (r *SchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	issueTypeIDs, _ := extractIssueTypeIDs(ctx, plan.IssueTypeIDs)

	body := apiIssueTypeSchemeCreate{
		Name:         plan.Name.ValueString(),
		IssueTypeIDs: issueTypeIDs,
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.DefaultIssueTypeID.IsNull() && !plan.DefaultIssueTypeID.IsUnknown() {
		body.DefaultIssueTypeID = plan.DefaultIssueTypeID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiIssueTypeScheme
	err := r.client.Post(ctx, "/rest/api/3/issuetypescheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate issue type scheme name",
					fmt.Sprintf("An issue type scheme with name %q already exists. Each issue type scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid issue type scheme configuration",
					fmt.Sprintf("The issue type scheme configuration for %q is invalid. Verify the issue type IDs and default issue type ID are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create issue type schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create issue type scheme",
			fmt.Sprintf("Could not create issue type scheme with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.DefaultIssueTypeID = types.StringValue(created.DefaultIssueTypeID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *SchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var scheme apiIssueTypeScheme
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/issuetypescheme/%s", state.ID.ValueString()), &scheme)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read issue type scheme",
			fmt.Sprintf("Could not read issue type scheme %q: %s. Verify the scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(scheme.ID)
	state.Name = types.StringValue(scheme.Name)
	state.Description = types.StringValue(scheme.Description)
	state.DefaultIssueTypeID = types.StringValue(scheme.DefaultIssueTypeID)
	// IssueTypeIDs are preserved from state since the GET response may not include them

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira issue type scheme.
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

	issueTypeIDs, _ := extractIssueTypeIDs(ctx, plan.IssueTypeIDs)

	body := apiIssueTypeSchemeUpdate{
		Name:         plan.Name.ValueString(),
		IssueTypeIDs: issueTypeIDs,
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.DefaultIssueTypeID.IsNull() && !plan.DefaultIssueTypeID.IsUnknown() {
		body.DefaultIssueTypeID = plan.DefaultIssueTypeID.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiIssueTypeScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/issuetypescheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Issue type scheme not found",
					fmt.Sprintf("Issue type scheme with ID %q not found. The scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid issue type scheme configuration",
					fmt.Sprintf("The issue type scheme update for ID %q is invalid. Verify the issue type IDs and default issue type ID are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update issue type schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update issue type scheme",
			fmt.Sprintf("Could not update issue type scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.DefaultIssueTypeID = types.StringValue(updated.DefaultIssueTypeID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira issue type scheme.
func (r *SchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/issuetypescheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete issue type scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete issue type scheme",
			fmt.Sprintf("Could not delete issue type scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing issue type scheme by ID.
func (r *SchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
