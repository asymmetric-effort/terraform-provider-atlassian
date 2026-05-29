// Package space implements Jira space (project) data sources including
// project versions (releases).
//
// VersionDataSource reads a Jira project version by ID from the
// Atlassian Cloud REST API (/rest/api/3/version).
package space

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure VersionDataSource satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &VersionDataSource{}

// apiVersionDS represents the JSON structure returned by the Atlassian version API.
type apiVersionDS struct {
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

// VersionDataSourceModel describes the version data source data model.
type VersionDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	SpaceID     types.String `tfsdk:"space_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	StartDate   types.String `tfsdk:"start_date"`
	ReleaseDate types.String `tfsdk:"release_date"`
	Released    types.Bool   `tfsdk:"released"`
	Archived    types.Bool   `tfsdk:"archived"`
}

// VersionDataSource implements the atlassian_jira_project_version data source.
type VersionDataSource struct {
	client *atlassian.Client
}

// NewVersionDataSource returns a new VersionDataSource instance for provider registration.
func NewVersionDataSource() datasource.DataSource {
	return &VersionDataSource{}
}

// Metadata returns the data source type name.
func (d *VersionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_version"
}

// Schema defines the schema for the jira project version data source.
func (d *VersionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira project version (release) from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the version.",
				Required:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The project ID that the version belongs to.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the version.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the version.",
				Computed:    true,
			},
			"start_date": schema.StringAttribute{
				Description: "The start date of the version.",
				Computed:    true,
			},
			"release_date": schema.StringAttribute{
				Description: "The release date of the version.",
				Computed:    true,
			},
			"released": schema.BoolAttribute{
				Description: "Whether the version has been released.",
				Computed:    true,
			},
			"archived": schema.BoolAttribute{
				Description: "Whether the version has been archived.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *VersionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves version data from the Atlassian API.
func (d *VersionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config VersionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	versionID := config.ID.ValueString()
	if versionID == "" {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"The id attribute must be specified to look up a Jira project version.",
		)
		return
	}

	var version apiVersionDS
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/version/%s", versionID), &version)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Version not found",
				fmt.Sprintf("Jira project version %q not found. Verify the ID is correct.", versionID),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read version",
			fmt.Sprintf("Could not read Jira project version %q: %s", versionID, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(version.ID)
	config.SpaceID = types.StringValue(version.ProjectID)
	config.Name = types.StringValue(version.Name)
	config.Description = types.StringValue(version.Description)
	config.StartDate = types.StringValue(version.StartDate)
	config.ReleaseDate = types.StringValue(version.ReleaseDate)
	config.Released = types.BoolValue(version.Released)
	config.Archived = types.BoolValue(version.Archived)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
