// Package repository implements the atlassian_bitbucket_repository managed resource.
//
// This resource manages Bitbucket Cloud repositories through the Atlassian
// REST API. It supports full CRUD operations and state import via
// workspace/slug. Uses the /2.0/repositories/{workspace}/{slug} endpoints.
package repository

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

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiRepository represents the JSON structure returned by the Bitbucket repository API.
type apiRepository struct {
	UUID        string           `json:"uuid"`
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	FullName    string           `json:"full_name"`
	Description string           `json:"description"`
	IsPrivate   bool             `json:"is_private"`
	ForkPolicy  string           `json:"fork_policy"`
	Language    string           `json:"language"`
	HasIssues   bool             `json:"has_issues"`
	HasWiki     bool             `json:"has_wiki"`
	MainBranch  *apiBranch       `json:"mainbranch,omitempty"`
	Links       *apiLinks        `json:"links,omitempty"`
	Owner       *apiOwner        `json:"owner,omitempty"`
	Workspace   *apiWorkspaceRef `json:"workspace,omitempty"`
}

// apiBranch represents a branch reference in the API.
type apiBranch struct {
	Name string `json:"name"`
}

// apiCloneLink represents a clone link entry.
type apiCloneLink struct {
	Href string `json:"href"`
	Name string `json:"name"`
}

// apiLinks represents the links section of a repository response.
type apiLinks struct {
	HTML  *apiHref       `json:"html,omitempty"`
	Clone []apiCloneLink `json:"clone,omitempty"`
}

// apiHref represents a single link.
type apiHref struct {
	Href string `json:"href"`
}

// apiOwner represents the owner in the API response.
type apiOwner struct {
	UUID string `json:"uuid"`
}

// apiWorkspaceRef represents a workspace reference in the API response.
type apiWorkspaceRef struct {
	Slug string `json:"slug"`
}

// apiRepoCreate represents the JSON body for creating/updating a repository.
type apiRepoCreate struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	IsPrivate   bool       `json:"is_private"`
	ForkPolicy  string     `json:"fork_policy,omitempty"`
	Language    string     `json:"language,omitempty"`
	HasIssues   bool       `json:"has_issues"`
	HasWiki     bool       `json:"has_wiki"`
	MainBranch  *apiBranch `json:"mainbranch,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Workspace     types.String `tfsdk:"workspace"`
	Slug          types.String `tfsdk:"slug"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	IsPrivate     types.Bool   `tfsdk:"is_private"`
	ForkPolicy    types.String `tfsdk:"fork_policy"`
	Language      types.String `tfsdk:"language"`
	DefaultBranch types.String `tfsdk:"default_branch"`
	HasIssues     types.Bool   `tfsdk:"has_issues"`
	HasWiki       types.Bool   `tfsdk:"has_wiki"`
	CloneSSH      types.String `tfsdk:"clone_ssh"`
	CloneHTTPS    types.String `tfsdk:"clone_https"`
	URL           types.String `tfsdk:"url"`
}

// Resource implements the atlassian_bitbucket_repository managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_repository"
}

// Schema defines the schema for the bitbucket repository resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Bitbucket Cloud repository.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The UUID of the repository, assigned by Bitbucket.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"workspace": schema.StringAttribute{
				Description: "The workspace slug that owns the repository.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"slug": schema.StringAttribute{
				Description: "The repository slug (URL-friendly name). Cannot be changed after creation.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the repository.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the repository.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"is_private": schema.BoolAttribute{
				Description: "Whether the repository is private.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"fork_policy": schema.StringAttribute{
				Description: "The fork policy for the repository (allow_forks, no_public_forks, no_forks).",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"language": schema.StringAttribute{
				Description: "The programming language of the repository.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_branch": schema.StringAttribute{
				Description: "The default branch of the repository.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"has_issues": schema.BoolAttribute{
				Description: "Whether the repository has the issue tracker enabled.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"has_wiki": schema.BoolAttribute{
				Description: "Whether the repository has the wiki enabled.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"clone_ssh": schema.StringAttribute{
				Description: "The SSH clone URL for the repository.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"clone_https": schema.StringAttribute{
				Description: "The HTTPS clone URL for the repository.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				Description: "The URL of the repository in Bitbucket Cloud.",
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

// extractCloneURL extracts the clone URL of the given type from the repository response.
func extractCloneURL(repo *apiRepository, name string) string {
	if repo.Links == nil {
		return ""
	}
	for _, link := range repo.Links.Clone {
		if link.Name == name {
			return link.Href
		}
	}
	return ""
}

// extractHTMLURL extracts the HTML URL from the repository response.
func extractHTMLURL(repo *apiRepository) string {
	if repo.Links != nil && repo.Links.HTML != nil {
		return repo.Links.HTML.Href
	}
	return ""
}

// mapRepoToModel maps an API repository to the Terraform model.
func mapRepoToModel(repo *apiRepository, workspace string) ResourceModel {
	defaultBranch := ""
	if repo.MainBranch != nil {
		defaultBranch = repo.MainBranch.Name
	}

	return ResourceModel{
		ID:            types.StringValue(repo.UUID),
		Workspace:     types.StringValue(workspace),
		Slug:          types.StringValue(repo.Slug),
		Name:          types.StringValue(repo.Name),
		Description:   types.StringValue(repo.Description),
		IsPrivate:     types.BoolValue(repo.IsPrivate),
		ForkPolicy:    types.StringValue(repo.ForkPolicy),
		Language:      types.StringValue(repo.Language),
		DefaultBranch: types.StringValue(defaultBranch),
		HasIssues:     types.BoolValue(repo.HasIssues),
		HasWiki:       types.BoolValue(repo.HasWiki),
		CloneSSH:      types.StringValue(extractCloneURL(repo, "ssh")),
		CloneHTTPS:    types.StringValue(extractCloneURL(repo, "https")),
		URL:           types.StringValue(extractHTMLURL(repo)),
	}
}

// repoPath returns the API path for a workspace/slug combination.
func repoPath(workspace, slug string) string {
	return fmt.Sprintf("/2.0/repositories/%s/%s", workspace, slug)
}

// Create provisions a new Bitbucket repository.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiRepoCreate{
		Name:      plan.Name.ValueString(),
		IsPrivate: plan.IsPrivate.ValueBool(),
		HasIssues: plan.HasIssues.ValueBool(),
		HasWiki:   plan.HasWiki.ValueBool(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.ForkPolicy.IsNull() && !plan.ForkPolicy.IsUnknown() {
		body.ForkPolicy = plan.ForkPolicy.ValueString()
	}
	if !plan.Language.IsNull() && !plan.Language.IsUnknown() {
		body.Language = plan.Language.ValueString()
	}
	if !plan.DefaultBranch.IsNull() && !plan.DefaultBranch.IsUnknown() && plan.DefaultBranch.ValueString() != "" {
		body.MainBranch = &apiBranch{Name: plan.DefaultBranch.ValueString()}
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiRepository
	apiPath := repoPath(plan.Workspace.ValueString(), plan.Slug.ValueString())
	// Bitbucket uses PUT to create repos at a specific slug
	err := r.client.Put(ctx, apiPath, bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate repository",
					fmt.Sprintf("A repository with slug %q already exists in workspace %q.", plan.Slug.ValueString(), plan.Workspace.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create repositories in this workspace.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create repository",
			fmt.Sprintf("Could not create repository %q in workspace %q: %s", plan.Slug.ValueString(), plan.Workspace.ValueString(), err.Error()),
		)
		return
	}

	model := mapRepoToModel(&created, plan.Workspace.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Read refreshes the Terraform state with the latest data from Bitbucket.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var repo apiRepository
	err := r.client.Get(ctx, repoPath(state.Workspace.ValueString(), state.Slug.ValueString()), &repo)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read repository",
			fmt.Sprintf("Could not read repository %q in workspace %q: %s. Verify the repository exists and has not been deleted.",
				state.Slug.ValueString(), state.Workspace.ValueString(), err.Error()),
		)
		return
	}

	model := mapRepoToModel(&repo, state.Workspace.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Update modifies an existing Bitbucket repository.
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

	body := apiRepoCreate{
		Name:      plan.Name.ValueString(),
		IsPrivate: plan.IsPrivate.ValueBool(),
		HasIssues: plan.HasIssues.ValueBool(),
		HasWiki:   plan.HasWiki.ValueBool(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.ForkPolicy.IsNull() && !plan.ForkPolicy.IsUnknown() {
		body.ForkPolicy = plan.ForkPolicy.ValueString()
	}
	if !plan.Language.IsNull() && !plan.Language.IsUnknown() {
		body.Language = plan.Language.ValueString()
	}
	if !plan.DefaultBranch.IsNull() && !plan.DefaultBranch.IsUnknown() && plan.DefaultBranch.ValueString() != "" {
		body.MainBranch = &apiBranch{Name: plan.DefaultBranch.ValueString()}
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiRepository
	err := r.client.Put(ctx, repoPath(state.Workspace.ValueString(), state.Slug.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Repository not found",
					fmt.Sprintf("Repository %q in workspace %q not found. The repository may have been deleted outside of Terraform.",
						state.Slug.ValueString(), state.Workspace.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update this repository.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update repository",
			fmt.Sprintf("Could not update repository %q in workspace %q: %s",
				state.Slug.ValueString(), state.Workspace.ValueString(), err.Error()),
		)
		return
	}

	model := mapRepoToModel(&updated, state.Workspace.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// Delete removes a Bitbucket repository.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, repoPath(state.Workspace.ValueString(), state.Slug.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete repository %q in workspace %q.",
						state.Slug.ValueString(), state.Workspace.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete repository",
			fmt.Sprintf("Could not delete repository %q in workspace %q: %s",
				state.Slug.ValueString(), state.Workspace.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing repository by ID (workspace/slug format).
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
