// Package repository implements the atlassian_bitbucket_repository_permission read-only data source.
//
// This data source reads Bitbucket repository permissions by ID from the
// Atlassian Cloud REST API.
package repository

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the PermissionDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &PermissionDataSource{}

// apiPermission represents the JSON structure returned by the Bitbucket permissions-config API.
type apiPermission struct {
	ID            string `json:"id"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Permission    string `json:"permission"`
}

// PermissionDataSourceModel describes the data source data model.
type PermissionDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Repository    types.String `tfsdk:"repository"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Permission    types.String `tfsdk:"permission"`
}

// PermissionDataSource implements the atlassian_bitbucket_repository_permission data source.
type PermissionDataSource struct {
	client *atlassian.Client
}

// NewPermissionDataSource returns a new PermissionDataSource instance for provider registration.
func NewPermissionDataSource() datasource.DataSource {
	return &PermissionDataSource{}
}

// Metadata returns the data source type name.
func (d *PermissionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_repository_permission"
}

// Schema defines the schema for the bitbucket repository permission data source.
func (d *PermissionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Bitbucket repository permission from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the permission.",
				Required:    true,
			},
			"repository": schema.StringAttribute{
				Description: "The repository in workspace/slug format (e.g., myworkspace/myrepo).",
				Required:    true,
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal (user, group).",
				Computed:    true,
			},
			"principal_id": schema.StringAttribute{
				Description: "The unique identifier of the principal.",
				Computed:    true,
			},
			"permission": schema.StringAttribute{
				Description: "The permission level (read, write, admin).",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *PermissionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// repoPath builds the API path prefix for a repository.
func repoPath(repository string) string {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("/2.0/repositories/%s/%s", parts[0], parts[1])
}

// Read retrieves repository permission data from the Bitbucket API.
func (d *PermissionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PermissionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := repoPath(config.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError(
			"Invalid repository format",
			"Repository must be in workspace/slug format (e.g., myworkspace/myrepo).",
		)
		return
	}

	var perm apiPermission
	err := d.client.Get(ctx, fmt.Sprintf("%s/permissions-config/%s", base, config.ID.ValueString()), &perm)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Repository permission not found",
				fmt.Sprintf("Repository permission %q not found in repository %q.", config.ID.ValueString(), config.Repository.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read repository permission",
			fmt.Sprintf("Could not read repository permission %q: %s", config.ID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(perm.ID)
	config.PrincipalType = types.StringValue(perm.PrincipalType)
	config.PrincipalID = types.StringValue(perm.PrincipalID)
	config.Permission = types.StringValue(perm.Permission)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
