// Package space implements the atlassian_jira_space read-only data source.
//
// This data source reads Jira spaces (projects) by ID or key from the
// Atlassian Cloud REST API.
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

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiSpace represents the JSON structure returned by the Atlassian project API.
type apiSpace struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	LeadAccountID  string `json:"leadAccountId,omitempty"`
	ProjectTypeKey string `json:"projectTypeKey"`
	Self           string `json:"self"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Key           types.String `tfsdk:"key"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	LeadAccountID types.String `tfsdk:"lead_account_id"`
	SpaceType     types.String `tfsdk:"space_type"`
	URL           types.String `tfsdk:"url"`
}

// DataSource implements the atlassian_jira_space data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_space"
}

// Schema defines the schema for the jira space data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira space (project) from Atlassian Cloud by ID or key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the space. Exactly one of id or key must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"key": schema.StringAttribute{
				Description: "The project key (e.g., PROJ). Exactly one of id or key must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the space.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the space.",
				Computed:    true,
			},
			"lead_account_id": schema.StringAttribute{
				Description: "The account ID of the space lead.",
				Computed:    true,
			},
			"space_type": schema.StringAttribute{
				Description: "The type of space (\"classic\" or \"next-gen\").",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the space in Atlassian Cloud.",
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

// projectTypeKeyToSpaceType converts the Atlassian API projectTypeKey to the user-facing space_type.
func projectTypeKeyToSpaceType(ptk string) string {
	if ptk == "software" {
		return "next-gen"
	}
	return "classic"
}

// Read retrieves space data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()
	hasKey := !config.Key.IsNull() && !config.Key.IsUnknown()

	if !hasID && !hasKey {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"Exactly one of id or key must be specified to look up a Jira space.",
		)
		return
	}

	identifier := config.ID.ValueString()
	if !hasID {
		identifier = config.Key.ValueString()
	}

	var space apiSpace
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/project/%s", identifier), &space)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Space not found",
				fmt.Sprintf("Jira space %q not found. Verify the ID or key is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read space",
			fmt.Sprintf("Could not read Jira space %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(space.ID)
	config.Key = types.StringValue(space.Key)
	config.Name = types.StringValue(space.Name)
	config.Description = types.StringValue(space.Description)
	config.LeadAccountID = types.StringValue(space.LeadAccountID)
	config.SpaceType = types.StringValue(projectTypeKeyToSpaceType(space.ProjectTypeKey))
	config.URL = types.StringValue(space.Self)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
