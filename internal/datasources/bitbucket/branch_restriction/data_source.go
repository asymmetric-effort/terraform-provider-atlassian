// Package branch_restriction implements the atlassian_bitbucket_branch_restriction read-only data source.
//
// This data source reads Bitbucket branch restrictions by ID from the
// Atlassian Cloud REST API.
package branch_restriction

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

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiBranchRestriction represents the JSON structure returned by the Bitbucket branch restrictions API.
type apiBranchRestriction struct {
	ID      int    `json:"id"`
	Kind    string `json:"kind"`
	Pattern string `json:"pattern"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Repository types.String `tfsdk:"repository"`
	Pattern    types.String `tfsdk:"pattern"`
	Kind       types.String `tfsdk:"kind"`
}

// DataSource implements the atlassian_bitbucket_branch_restriction data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_branch_restriction"
}

// Schema defines the schema for the bitbucket branch restriction data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Bitbucket branch restriction from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the branch restriction.",
				Required:    true,
			},
			"repository": schema.StringAttribute{
				Description: "The repository in workspace/slug format (e.g., myworkspace/myrepo).",
				Required:    true,
			},
			"pattern": schema.StringAttribute{
				Description: "The branch pattern being restricted.",
				Computed:    true,
			},
			"kind": schema.StringAttribute{
				Description: "The kind of restriction (push, delete, force, merge).",
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

// repoPath builds the API path prefix for a repository.
func repoPath(repository string) string {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("/2.0/repositories/%s/%s", parts[0], parts[1])
}

// Read retrieves branch restriction data from the Bitbucket API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
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

	var restriction apiBranchRestriction
	err := d.client.Get(ctx, fmt.Sprintf("%s/branch-restrictions/%s", base, config.ID.ValueString()), &restriction)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Branch restriction not found",
				fmt.Sprintf("Branch restriction %q not found in repository %q.", config.ID.ValueString(), config.Repository.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read branch restriction",
			fmt.Sprintf("Could not read branch restriction %q: %s", config.ID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(fmt.Sprintf("%d", restriction.ID))
	config.Pattern = types.StringValue(restriction.Pattern)
	config.Kind = types.StringValue(restriction.Kind)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
