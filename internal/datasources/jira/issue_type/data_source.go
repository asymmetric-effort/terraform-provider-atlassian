// Package issuetype implements the atlassian_jira_issue_type read-only data source.
//
// This data source reads Jira issue types by ID from the Atlassian Cloud REST API.
package issuetype

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

// apiIssueType represents the JSON structure returned by the Atlassian issue type API.
type apiIssueType struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	IconURL        string `json:"iconUrl,omitempty"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int64  `json:"hierarchyLevel"`
	Self           string `json:"self"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	IconURL        types.String `tfsdk:"icon_url"`
	Subtask        types.Bool   `tfsdk:"subtask"`
	HierarchyLevel types.Int64  `tfsdk:"hierarchy_level"`
}

// DataSource implements the atlassian_jira_issue_type data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type"
}

// Schema defines the schema for the jira issue type data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira issue type from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the issue type.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the issue type.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the issue type.",
				Computed:    true,
			},
			"icon_url": schema.StringAttribute{
				Description: "The URL of the issue type icon.",
				Computed:    true,
			},
			"subtask": schema.BoolAttribute{
				Description: "Whether this issue type represents a subtask.",
				Computed:    true,
			},
			"hierarchy_level": schema.Int64Attribute{
				Description: "The hierarchy level of the issue type.",
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

// Read retrieves issue type data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()
	if identifier == "" {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"The id attribute must be specified to look up a Jira issue type.",
		)
		return
	}

	var issueType apiIssueType
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/issuetype/%s", identifier), &issueType)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Issue type not found",
				fmt.Sprintf("Jira issue type %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read issue type",
			fmt.Sprintf("Could not read Jira issue type %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(issueType.ID)
	config.Name = types.StringValue(issueType.Name)
	config.Description = types.StringValue(issueType.Description)
	config.IconURL = types.StringValue(issueType.IconURL)
	config.Subtask = types.BoolValue(issueType.Subtask)
	config.HierarchyLevel = types.Int64Value(issueType.HierarchyLevel)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
