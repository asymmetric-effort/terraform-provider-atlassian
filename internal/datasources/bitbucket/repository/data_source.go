// Package repository implements the atlassian_bitbucket_repository read-only data source.
//
// This data source reads Bitbucket Cloud repositories by workspace and slug
// from the Atlassian REST API.
package repository

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

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

// apiWorkspaceRef represents a workspace reference in the API response.
type apiWorkspaceRef struct {
	Slug string `json:"slug"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
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

// DataSource implements the atlassian_bitbucket_repository data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_repository"
}

// Schema defines the schema for the bitbucket repository data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Bitbucket Cloud repository by workspace and slug.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The UUID of the repository.",
				Computed:    true,
			},
			"workspace": schema.StringAttribute{
				Description: "The workspace slug that owns the repository.",
				Required:    true,
			},
			"slug": schema.StringAttribute{
				Description: "The repository slug.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the repository.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the repository.",
				Computed:    true,
			},
			"is_private": schema.BoolAttribute{
				Description: "Whether the repository is private.",
				Computed:    true,
			},
			"fork_policy": schema.StringAttribute{
				Description: "The fork policy for the repository.",
				Computed:    true,
			},
			"language": schema.StringAttribute{
				Description: "The programming language of the repository.",
				Computed:    true,
			},
			"default_branch": schema.StringAttribute{
				Description: "The default branch of the repository.",
				Computed:    true,
			},
			"has_issues": schema.BoolAttribute{
				Description: "Whether the repository has the issue tracker enabled.",
				Computed:    true,
			},
			"has_wiki": schema.BoolAttribute{
				Description: "Whether the repository has the wiki enabled.",
				Computed:    true,
			},
			"clone_ssh": schema.StringAttribute{
				Description: "The SSH clone URL for the repository.",
				Computed:    true,
			},
			"clone_https": schema.StringAttribute{
				Description: "The HTTPS clone URL for the repository.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the repository in Bitbucket Cloud.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.client = client
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

// Read retrieves repository data from the Bitbucket API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	workspace := config.Workspace.ValueString()
	slug := config.Slug.ValueString()

	var repo apiRepository
	err := d.client.Get(ctx, fmt.Sprintf("/2.0/repositories/%s/%s", workspace, slug), &repo)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Repository not found",
				fmt.Sprintf("Bitbucket repository %q in workspace %q not found. Verify the workspace and slug are correct.",
					slug, workspace),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read repository",
			fmt.Sprintf("Could not read Bitbucket repository %q in workspace %q: %s", slug, workspace, err.Error()),
		)
		return
	}

	defaultBranch := ""
	if repo.MainBranch != nil {
		defaultBranch = repo.MainBranch.Name
	}

	config.ID = types.StringValue(repo.UUID)
	config.Name = types.StringValue(repo.Name)
	config.Description = types.StringValue(repo.Description)
	config.IsPrivate = types.BoolValue(repo.IsPrivate)
	config.ForkPolicy = types.StringValue(repo.ForkPolicy)
	config.Language = types.StringValue(repo.Language)
	config.DefaultBranch = types.StringValue(defaultBranch)
	config.HasIssues = types.BoolValue(repo.HasIssues)
	config.HasWiki = types.BoolValue(repo.HasWiki)
	config.CloneSSH = types.StringValue(extractCloneURL(&repo, "ssh"))
	config.CloneHTTPS = types.StringValue(extractCloneURL(&repo, "https"))
	config.URL = types.StringValue(extractHTMLURL(&repo))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
