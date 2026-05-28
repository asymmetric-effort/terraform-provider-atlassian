// Package deployment implements the atlassian_bitbucket_deployment read-only data source.
//
// This data source reads Bitbucket deployment environments by UUID from the
// Atlassian Cloud REST API.
package deployment

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

// apiEnvironmentType represents the nested environment_type in the Bitbucket API.
type apiEnvironmentType struct {
	Name string `json:"name"`
}

// apiDeployment represents the JSON structure returned by the Bitbucket environments API.
type apiDeployment struct {
	UUID            string             `json:"uuid"`
	Name            string             `json:"name"`
	EnvironmentType apiEnvironmentType `json:"environment_type"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Repository      types.String `tfsdk:"repository"`
	Name            types.String `tfsdk:"name"`
	EnvironmentType types.String `tfsdk:"environment_type"`
}

// DataSource implements the atlassian_bitbucket_deployment data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_deployment"
}

// Schema defines the schema for the bitbucket deployment data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Bitbucket deployment environment from Atlassian Cloud by UUID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier (UUID) of the deployment environment.",
				Required:    true,
			},
			"repository": schema.StringAttribute{
				Description: "The repository in workspace/slug format (e.g., myworkspace/myrepo).",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the deployment environment.",
				Computed:    true,
			},
			"environment_type": schema.StringAttribute{
				Description: "The type of deployment environment (test, staging, production).",
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

// Read retrieves deployment environment data from the Bitbucket API.
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

	var env apiDeployment
	err := d.client.Get(ctx, fmt.Sprintf("%s/environments/%s", base, config.ID.ValueString()), &env)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Deployment environment not found",
				fmt.Sprintf("Deployment environment %q not found in repository %q.", config.ID.ValueString(), config.Repository.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read deployment environment",
			fmt.Sprintf("Could not read deployment environment %q: %s", config.ID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(env.UUID)
	config.Name = types.StringValue(env.Name)
	config.EnvironmentType = types.StringValue(env.EnvironmentType.Name)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
