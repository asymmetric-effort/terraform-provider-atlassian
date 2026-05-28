// Package pipeline implements the atlassian_bitbucket_pipeline read-only data source.
//
// This data source reads Bitbucket pipeline configuration from the
// Atlassian Cloud REST API.
package pipeline

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

// apiPipeline represents the JSON structure returned by the Bitbucket pipelines_config API.
type apiPipeline struct {
	Enabled bool `json:"enabled"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	Repository types.String `tfsdk:"repository"`
	Enabled    types.Bool   `tfsdk:"enabled"`
}

// DataSource implements the atlassian_bitbucket_pipeline data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_pipeline"
}

// Schema defines the schema for the bitbucket pipeline data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads Bitbucket pipeline configuration from Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The identifier (same as repository).",
				Computed:    true,
			},
			"repository": schema.StringAttribute{
				Description: "The repository in workspace/slug format (e.g., myworkspace/myrepo).",
				Required:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether pipelines are enabled for this repository.",
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

// Read retrieves pipeline configuration data from the Bitbucket API.
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

	var pipeline apiPipeline
	err := d.client.Get(ctx, base+"/pipelines_config", &pipeline)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Pipeline configuration not found",
				fmt.Sprintf("Pipeline configuration not found for repository %q.", config.Repository.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read pipeline configuration",
			fmt.Sprintf("Could not read pipeline configuration: %s", err.Error()),
		)
		return
	}

	config.ID = types.StringValue(config.Repository.ValueString())
	config.Enabled = types.BoolValue(pipeline.Enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
