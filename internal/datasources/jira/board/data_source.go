// Package board implements the atlassian_jira_board read-only data source.
//
// This data source reads Jira boards by ID from the Atlassian Cloud REST API.
package board

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

// apiBoard represents the JSON structure returned by the Atlassian board API.
type apiBoard struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	SpaceID string `json:"spaceId"`
	Self    string `json:"self"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	SpaceID types.String `tfsdk:"space_id"`
}

// DataSource implements the atlassian_jira_board data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_board"
}

// Schema defines the schema for the jira board data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira board from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the board.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the board.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of the board (scrum or kanban).",
				Computed:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the space (project) associated with this board.",
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

// Read retrieves board data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var b apiBoard
	err := d.client.Get(ctx, fmt.Sprintf("/rest/agile/1.0/board/%s", identifier), &b)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Board not found",
				fmt.Sprintf("Jira board %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read board",
			fmt.Sprintf("Could not read Jira board %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(b.ID)
	config.Name = types.StringValue(b.Name)
	config.Type = types.StringValue(b.Type)
	config.SpaceID = types.StringValue(b.SpaceID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
