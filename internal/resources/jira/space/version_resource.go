// Package space implements Jira space (project) resources including
// project versions (releases).
//
// VersionResource manages Jira project versions through the Atlassian
// Cloud REST API (/rest/api/3/version). It supports full CRUD operations
// and state import via version ID.
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure VersionResource satisfies framework interfaces.
var (
	_ resource.Resource                = &VersionResource{}
	_ resource.ResourceWithImportState = &VersionResource{}
)

// apiVersion represents the JSON structure returned by the Atlassian version API.
type apiVersion struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
	Self        string `json:"self,omitempty"`
}

// apiVersionCreate represents the JSON body for creating a version.
type apiVersionCreate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ProjectID   string `json:"projectId"`
	StartDate   string `json:"startDate,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
}

// apiVersionUpdate represents the JSON body for updating a version.
type apiVersionUpdate struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
	Released    bool   `json:"released"`
	Archived    bool   `json:"archived"`
}

// VersionResourceModel describes the version resource data model.
type VersionResourceModel struct {
	ID          types.String `tfsdk:"id"`
	SpaceID     types.String `tfsdk:"space_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	StartDate   types.String `tfsdk:"start_date"`
	ReleaseDate types.String `tfsdk:"release_date"`
	Released    types.Bool   `tfsdk:"released"`
	Archived    types.Bool   `tfsdk:"archived"`
}

// VersionResource implements the atlassian_jira_project_version managed resource.
type VersionResource struct {
	client *atlassian.Client
}

// NewVersionResource returns a new VersionResource instance for provider registration.
func NewVersionResource() resource.Resource {
	return &VersionResource{}
}

// Metadata returns the resource type name.
func (r *VersionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_version"
}

// Schema defines the schema for the jira project version resource.
func (r *VersionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira project version (release) in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the version, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The project ID that the version belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the version. Must be unique within the project.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the version.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"start_date": schema.StringAttribute{
				Description: "The start date of the version in ISO 8601 format (e.g. 2024-01-01).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"release_date": schema.StringAttribute{
				Description: "The release date of the version in ISO 8601 format (e.g. 2024-06-01).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"released": schema.BoolAttribute{
				Description: "Whether the version has been released.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"archived": schema.BoolAttribute{
				Description: "Whether the version has been archived.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *VersionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira project version.
func (r *VersionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan VersionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiVersionCreate{
		Name:      plan.Name.ValueString(),
		ProjectID: plan.SpaceID.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.StartDate.IsNull() && !plan.StartDate.IsUnknown() {
		body.StartDate = plan.StartDate.ValueString()
	}
	if !plan.ReleaseDate.IsNull() && !plan.ReleaseDate.IsUnknown() {
		body.ReleaseDate = plan.ReleaseDate.ValueString()
	}
	if !plan.Released.IsNull() && !plan.Released.IsUnknown() {
		body.Released = plan.Released.ValueBool()
	}
	if !plan.Archived.IsNull() && !plan.Archived.IsUnknown() {
		body.Archived = plan.Archived.ValueBool()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiVersion
	err := r.client.Post(ctx, "/rest/api/3/version", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate version name",
					fmt.Sprintf("A version with name %q already exists in project %q. Each version name must be unique within a project.",
						plan.Name.ValueString(), plan.SpaceID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create versions. Ensure the service account has project admin privileges.",
				)
				return
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Project not found",
					fmt.Sprintf("Project with ID %q not found. Verify the space_id is correct.", plan.SpaceID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create version",
			fmt.Sprintf("Could not create version %q in project %q: %s",
				plan.Name.ValueString(), plan.SpaceID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.StartDate = types.StringValue(created.StartDate)
	plan.ReleaseDate = types.StringValue(created.ReleaseDate)
	plan.Released = types.BoolValue(created.Released)
	plan.Archived = types.BoolValue(created.Archived)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *VersionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state VersionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var version apiVersion
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/version/%s", state.ID.ValueString()), &version)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read version",
			fmt.Sprintf("Could not read version %q: %s. Verify the version exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(version.ID)
	state.Name = types.StringValue(version.Name)
	state.Description = types.StringValue(version.Description)
	state.StartDate = types.StringValue(version.StartDate)
	state.ReleaseDate = types.StringValue(version.ReleaseDate)
	state.Released = types.BoolValue(version.Released)
	state.Archived = types.BoolValue(version.Archived)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira project version.
func (r *VersionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VersionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state VersionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiVersionUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.StartDate.IsNull() && !plan.StartDate.IsUnknown() {
		body.StartDate = plan.StartDate.ValueString()
	}
	if !plan.ReleaseDate.IsNull() && !plan.ReleaseDate.IsUnknown() {
		body.ReleaseDate = plan.ReleaseDate.ValueString()
	}
	if !plan.Released.IsNull() && !plan.Released.IsUnknown() {
		body.Released = plan.Released.ValueBool()
	}
	if !plan.Archived.IsNull() && !plan.Archived.IsUnknown() {
		body.Archived = plan.Archived.ValueBool()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiVersion
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/version/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Version not found",
					fmt.Sprintf("Version with ID %q not found. The version may have been deleted outside of Terraform.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update versions. Ensure the service account has project admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update version",
			fmt.Sprintf("Could not update version with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.StartDate = types.StringValue(updated.StartDate)
	plan.ReleaseDate = types.StringValue(updated.ReleaseDate)
	plan.Released = types.BoolValue(updated.Released)
	plan.Archived = types.BoolValue(updated.Archived)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira project version.
func (r *VersionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state VersionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/version/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete version %q. "+
						"Ensure the service account has project admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete version",
			fmt.Sprintf("Could not delete version with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing version by ID.
func (r *VersionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
