// Package space implements the atlassian_jira_space managed resource.
//
// This resource manages Jira spaces (projects) through the Atlassian
// Cloud REST API. It supports full CRUD operations and state import
// via space ID or key. Internally, Atlassian uses "project" endpoints
// but this provider exposes them as "spaces" per the provider naming
// convention.
package space

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

// apiSpace represents the JSON structure returned by the Atlassian project API.
type apiSpace struct {
	ID                    string `json:"id"`
	Key                   string `json:"key"`
	Name                  string `json:"name"`
	Description           string `json:"description"`
	LeadAccountID         string `json:"leadAccountId,omitempty"`
	ProjectTypeKey        string `json:"projectTypeKey"`
	ProjectTemplateKey    string `json:"projectTemplateKey,omitempty"`
	AvatarID              int64  `json:"avatarId,omitempty"`
	CategoryID            int64  `json:"categoryId,omitempty"`
	AssigneeType          string `json:"assigneeType,omitempty"`
	IssueTypeScheme       int64  `json:"issueTypeScheme,omitempty"`
	IssueTypeScreenScheme int64  `json:"issueTypeScreenScheme,omitempty"`
	WorkflowScheme        int64  `json:"workflowScheme,omitempty"`
	NotificationScheme    int64  `json:"notificationScheme,omitempty"`
	PermissionScheme      int64  `json:"permissionScheme,omitempty"`
	IssueSecurityScheme   int64  `json:"issueSecurityScheme,omitempty"`
	FieldScheme           int64  `json:"fieldScheme,omitempty"`
	Self                  string `json:"self"`
}

// apiSpaceCreate represents the JSON body for creating a space.
type apiSpaceCreate struct {
	Key                   string `json:"key"`
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	LeadAccountID         string `json:"leadAccountId,omitempty"`
	ProjectTypeKey        string `json:"projectTypeKey"`
	ProjectTemplateKey    string `json:"projectTemplateKey,omitempty"`
	AvatarID              int64  `json:"avatarId,omitempty"`
	CategoryID            int64  `json:"categoryId,omitempty"`
	AssigneeType          string `json:"assigneeType,omitempty"`
	IssueTypeScheme       int64  `json:"issueTypeScheme,omitempty"`
	IssueTypeScreenScheme int64  `json:"issueTypeScreenScheme,omitempty"`
	WorkflowScheme        int64  `json:"workflowScheme,omitempty"`
	NotificationScheme    int64  `json:"notificationScheme,omitempty"`
	PermissionScheme      int64  `json:"permissionScheme,omitempty"`
	IssueSecurityScheme   int64  `json:"issueSecurityScheme,omitempty"`
	FieldScheme           int64  `json:"fieldScheme,omitempty"`
}

// apiSpaceUpdate represents the JSON body for updating a space.
type apiSpaceUpdate struct {
	Name                  string `json:"name,omitempty"`
	Description           string `json:"description,omitempty"`
	LeadAccountID         string `json:"leadAccountId,omitempty"`
	ProjectTypeKey        string `json:"projectTypeKey,omitempty"`
	ProjectTemplateKey    string `json:"projectTemplateKey,omitempty"`
	AvatarID              int64  `json:"avatarId,omitempty"`
	CategoryID            int64  `json:"categoryId,omitempty"`
	AssigneeType          string `json:"assigneeType,omitempty"`
	IssueTypeScheme       int64  `json:"issueTypeScheme,omitempty"`
	IssueTypeScreenScheme int64  `json:"issueTypeScreenScheme,omitempty"`
	WorkflowScheme        int64  `json:"workflowScheme,omitempty"`
	NotificationScheme    int64  `json:"notificationScheme,omitempty"`
	PermissionScheme      int64  `json:"permissionScheme,omitempty"`
	IssueSecurityScheme   int64  `json:"issueSecurityScheme,omitempty"`
	FieldScheme           int64  `json:"fieldScheme,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID                    types.String `tfsdk:"id"`
	Key                   types.String `tfsdk:"key"`
	Name                  types.String `tfsdk:"name"`
	Description           types.String `tfsdk:"description"`
	LeadAccountID         types.String `tfsdk:"lead_account_id"`
	SpaceType             types.String `tfsdk:"space_type"`
	ProjectTemplateKey    types.String `tfsdk:"project_template_key"`
	AvatarID              types.Int64  `tfsdk:"avatar_id"`
	CategoryID            types.Int64  `tfsdk:"category_id"`
	AssigneeType          types.String `tfsdk:"assignee_type"`
	IssueTypeScheme       types.Int64  `tfsdk:"issue_type_scheme"`
	IssueTypeScreenScheme types.Int64  `tfsdk:"issue_type_screen_scheme"`
	WorkflowScheme        types.Int64  `tfsdk:"workflow_scheme"`
	NotificationScheme    types.Int64  `tfsdk:"notification_scheme"`
	PermissionScheme      types.Int64  `tfsdk:"permission_scheme"`
	IssueSecurityScheme   types.Int64  `tfsdk:"issue_security_scheme"`
	FieldScheme           types.Int64  `tfsdk:"field_scheme"`
	URL                   types.String `tfsdk:"url"`
	SelfURL               types.String `tfsdk:"self_url"`
	BrowseURL             types.String `tfsdk:"browse_url"`
}

// Resource implements the atlassian_jira_space managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_space"
}

// Schema defines the schema for the jira space resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira space (project) in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the space, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				Description: "The project key (e.g., PROJ). Must be unique and cannot be changed after creation.",
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
				Description: "A description of the space.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"lead_account_id": schema.StringAttribute{
				Description: "The account ID of the space lead.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_type": schema.StringAttribute{
				Description: "The type of space. Must be \"classic\" or \"next-gen\".",
				Required:    true,
			},
			"project_template_key": schema.StringAttribute{
				Description: "The project template key used when creating the space.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"avatar_id": schema.Int64Attribute{
				Description: "The ID of the avatar for the space.",
				Optional:    true,
				Computed:    true,
			},
			"category_id": schema.Int64Attribute{
				Description: "The ID of the project category for the space.",
				Optional:    true,
				Computed:    true,
			},
			"assignee_type": schema.StringAttribute{
				Description: "The default assignee type for the space. Must be \"PROJECT_LEAD\" or \"UNASSIGNED\".",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"issue_type_scheme": schema.Int64Attribute{
				Description: "The ID of the issue type scheme for the space.",
				Optional:    true,
				Computed:    true,
			},
			"issue_type_screen_scheme": schema.Int64Attribute{
				Description: "The ID of the issue type screen scheme for the space.",
				Optional:    true,
				Computed:    true,
			},
			"workflow_scheme": schema.Int64Attribute{
				Description: "The ID of the workflow scheme for the space.",
				Optional:    true,
				Computed:    true,
			},
			"notification_scheme": schema.Int64Attribute{
				Description: "The ID of the notification scheme for the space.",
				Optional:    true,
				Computed:    true,
			},
			"permission_scheme": schema.Int64Attribute{
				Description: "The ID of the permission scheme for the space.",
				Optional:    true,
				Computed:    true,
			},
			"issue_security_scheme": schema.Int64Attribute{
				Description: "The ID of the issue security scheme for the space.",
				Optional:    true,
				Computed:    true,
			},
			"field_scheme": schema.Int64Attribute{
				Description: "The ID of the field scheme for the space.",
				Optional:    true,
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the space in Atlassian Cloud.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"self_url": schema.StringAttribute{
				Description: "The self URL of the space resource in the Atlassian API.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"browse_url": schema.StringAttribute{
				Description: "The browser-accessible URL of the space.",
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

// spaceTypeToProjectTypeKey converts the user-facing space_type to the Atlassian API projectTypeKey.
func spaceTypeToProjectTypeKey(spaceType string) string {
	if spaceType == "next-gen" {
		return "software"
	}
	return "business"
}

// projectTypeKeyToSpaceType converts the Atlassian API projectTypeKey to the user-facing space_type.
func projectTypeKeyToSpaceType(ptk string) string {
	if ptk == "software" {
		return "next-gen"
	}
	return "classic"
}

// Create provisions a new Jira space.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiSpaceCreate{
		Key:            plan.Key.ValueString(),
		Name:           plan.Name.ValueString(),
		ProjectTypeKey: spaceTypeToProjectTypeKey(plan.SpaceType.ValueString()),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.LeadAccountID.IsNull() && !plan.LeadAccountID.IsUnknown() {
		body.LeadAccountID = plan.LeadAccountID.ValueString()
	}
	if !plan.ProjectTemplateKey.IsNull() && !plan.ProjectTemplateKey.IsUnknown() {
		body.ProjectTemplateKey = plan.ProjectTemplateKey.ValueString()
	}
	if !plan.AvatarID.IsNull() && !plan.AvatarID.IsUnknown() {
		body.AvatarID = plan.AvatarID.ValueInt64()
	}
	if !plan.CategoryID.IsNull() && !plan.CategoryID.IsUnknown() {
		body.CategoryID = plan.CategoryID.ValueInt64()
	}
	if !plan.AssigneeType.IsNull() && !plan.AssigneeType.IsUnknown() {
		body.AssigneeType = plan.AssigneeType.ValueString()
	}
	if !plan.IssueTypeScheme.IsNull() && !plan.IssueTypeScheme.IsUnknown() {
		body.IssueTypeScheme = plan.IssueTypeScheme.ValueInt64()
	}
	if !plan.IssueTypeScreenScheme.IsNull() && !plan.IssueTypeScreenScheme.IsUnknown() {
		body.IssueTypeScreenScheme = plan.IssueTypeScreenScheme.ValueInt64()
	}
	if !plan.WorkflowScheme.IsNull() && !plan.WorkflowScheme.IsUnknown() {
		body.WorkflowScheme = plan.WorkflowScheme.ValueInt64()
	}
	if !plan.NotificationScheme.IsNull() && !plan.NotificationScheme.IsUnknown() {
		body.NotificationScheme = plan.NotificationScheme.ValueInt64()
	}
	if !plan.PermissionScheme.IsNull() && !plan.PermissionScheme.IsUnknown() {
		body.PermissionScheme = plan.PermissionScheme.ValueInt64()
	}
	if !plan.IssueSecurityScheme.IsNull() && !plan.IssueSecurityScheme.IsUnknown() {
		body.IssueSecurityScheme = plan.IssueSecurityScheme.ValueInt64()
	}
	if !plan.FieldScheme.IsNull() && !plan.FieldScheme.IsUnknown() {
		body.FieldScheme = plan.FieldScheme.ValueInt64()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiSpace
	err := r.client.Post(ctx, "/rest/api/3/project", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate space key",
					fmt.Sprintf("A space with key %q already exists. Each space key must be unique within the Atlassian organization.", plan.Key.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create spaces. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create space",
			fmt.Sprintf("Could not create space with key %q: %s", plan.Key.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Key = types.StringValue(created.Key)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.LeadAccountID = types.StringValue(created.LeadAccountID)
	plan.SpaceType = types.StringValue(projectTypeKeyToSpaceType(created.ProjectTypeKey))
	plan.ProjectTemplateKey = types.StringValue(created.ProjectTemplateKey)
	plan.AvatarID = types.Int64Value(created.AvatarID)
	plan.CategoryID = types.Int64Value(created.CategoryID)
	plan.AssigneeType = types.StringValue(created.AssigneeType)
	plan.IssueTypeScheme = types.Int64Value(created.IssueTypeScheme)
	plan.IssueTypeScreenScheme = types.Int64Value(created.IssueTypeScreenScheme)
	plan.WorkflowScheme = types.Int64Value(created.WorkflowScheme)
	plan.NotificationScheme = types.Int64Value(created.NotificationScheme)
	plan.PermissionScheme = types.Int64Value(created.PermissionScheme)
	plan.IssueSecurityScheme = types.Int64Value(created.IssueSecurityScheme)
	plan.FieldScheme = types.Int64Value(created.FieldScheme)
	plan.URL = types.StringValue(created.Self)
	plan.SelfURL = types.StringValue(created.Self)
	plan.BrowseURL = types.StringValue(browseURL(created.Self, created.Key))

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

	var space apiSpace
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/project/%s", identifier), &space)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read space",
			fmt.Sprintf("Could not read space %q: %s. Verify the space exists and has not been deleted.",
				identifier, err.Error()),
		)
		return
	}

	state.ID = types.StringValue(space.ID)
	state.Key = types.StringValue(space.Key)
	state.Name = types.StringValue(space.Name)
	state.Description = types.StringValue(space.Description)
	state.LeadAccountID = types.StringValue(space.LeadAccountID)
	state.SpaceType = types.StringValue(projectTypeKeyToSpaceType(space.ProjectTypeKey))
	state.ProjectTemplateKey = types.StringValue(space.ProjectTemplateKey)
	state.AvatarID = types.Int64Value(space.AvatarID)
	state.CategoryID = types.Int64Value(space.CategoryID)
	state.AssigneeType = types.StringValue(space.AssigneeType)
	state.IssueTypeScheme = types.Int64Value(space.IssueTypeScheme)
	state.IssueTypeScreenScheme = types.Int64Value(space.IssueTypeScreenScheme)
	state.WorkflowScheme = types.Int64Value(space.WorkflowScheme)
	state.NotificationScheme = types.Int64Value(space.NotificationScheme)
	state.PermissionScheme = types.Int64Value(space.PermissionScheme)
	state.IssueSecurityScheme = types.Int64Value(space.IssueSecurityScheme)
	state.FieldScheme = types.Int64Value(space.FieldScheme)
	state.URL = types.StringValue(space.Self)
	state.SelfURL = types.StringValue(space.Self)
	state.BrowseURL = types.StringValue(browseURL(space.Self, space.Key))

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira space.
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
		Name:           plan.Name.ValueString(),
		ProjectTypeKey: spaceTypeToProjectTypeKey(plan.SpaceType.ValueString()),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.LeadAccountID.IsNull() && !plan.LeadAccountID.IsUnknown() {
		body.LeadAccountID = plan.LeadAccountID.ValueString()
	}
	if !plan.ProjectTemplateKey.IsNull() && !plan.ProjectTemplateKey.IsUnknown() {
		body.ProjectTemplateKey = plan.ProjectTemplateKey.ValueString()
	}
	if !plan.AvatarID.IsNull() && !plan.AvatarID.IsUnknown() {
		body.AvatarID = plan.AvatarID.ValueInt64()
	}
	if !plan.CategoryID.IsNull() && !plan.CategoryID.IsUnknown() {
		body.CategoryID = plan.CategoryID.ValueInt64()
	}
	if !plan.AssigneeType.IsNull() && !plan.AssigneeType.IsUnknown() {
		body.AssigneeType = plan.AssigneeType.ValueString()
	}
	if !plan.IssueTypeScheme.IsNull() && !plan.IssueTypeScheme.IsUnknown() {
		body.IssueTypeScheme = plan.IssueTypeScheme.ValueInt64()
	}
	if !plan.IssueTypeScreenScheme.IsNull() && !plan.IssueTypeScreenScheme.IsUnknown() {
		body.IssueTypeScreenScheme = plan.IssueTypeScreenScheme.ValueInt64()
	}
	if !plan.WorkflowScheme.IsNull() && !plan.WorkflowScheme.IsUnknown() {
		body.WorkflowScheme = plan.WorkflowScheme.ValueInt64()
	}
	if !plan.NotificationScheme.IsNull() && !plan.NotificationScheme.IsUnknown() {
		body.NotificationScheme = plan.NotificationScheme.ValueInt64()
	}
	if !plan.PermissionScheme.IsNull() && !plan.PermissionScheme.IsUnknown() {
		body.PermissionScheme = plan.PermissionScheme.ValueInt64()
	}
	if !plan.IssueSecurityScheme.IsNull() && !plan.IssueSecurityScheme.IsUnknown() {
		body.IssueSecurityScheme = plan.IssueSecurityScheme.ValueInt64()
	}
	if !plan.FieldScheme.IsNull() && !plan.FieldScheme.IsUnknown() {
		body.FieldScheme = plan.FieldScheme.ValueInt64()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiSpace
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/project/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Space not found",
					fmt.Sprintf("Space with ID %q not found. The space may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update spaces. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update space",
			fmt.Sprintf("Could not update space with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Key = types.StringValue(updated.Key)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.LeadAccountID = types.StringValue(updated.LeadAccountID)
	plan.SpaceType = types.StringValue(projectTypeKeyToSpaceType(updated.ProjectTypeKey))
	plan.ProjectTemplateKey = types.StringValue(updated.ProjectTemplateKey)
	plan.AvatarID = types.Int64Value(updated.AvatarID)
	plan.CategoryID = types.Int64Value(updated.CategoryID)
	plan.AssigneeType = types.StringValue(updated.AssigneeType)
	plan.IssueTypeScheme = types.Int64Value(updated.IssueTypeScheme)
	plan.IssueTypeScreenScheme = types.Int64Value(updated.IssueTypeScreenScheme)
	plan.WorkflowScheme = types.Int64Value(updated.WorkflowScheme)
	plan.NotificationScheme = types.Int64Value(updated.NotificationScheme)
	plan.PermissionScheme = types.Int64Value(updated.PermissionScheme)
	plan.IssueSecurityScheme = types.Int64Value(updated.IssueSecurityScheme)
	plan.FieldScheme = types.Int64Value(updated.FieldScheme)
	plan.URL = types.StringValue(updated.Self)
	plan.SelfURL = types.StringValue(updated.Self)
	plan.BrowseURL = types.StringValue(browseURL(updated.Self, updated.Key))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// browseURL constructs a browser-accessible URL from the API self URL
// and the space key. The self URL has the form
// https://example.atlassian.net/rest/api/3/project/ID
// and the browse URL is https://example.atlassian.net/browse/KEY.
func browseURL(selfURL, key string) string {
	idx := strings.Index(selfURL, "/rest/api/")
	if idx < 0 {
		return ""
	}
	return selfURL[:idx] + "/browse/" + key
}

// Delete removes a Jira space.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/project/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete space %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete space",
			fmt.Sprintf("Could not delete space with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing space by ID or key.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
