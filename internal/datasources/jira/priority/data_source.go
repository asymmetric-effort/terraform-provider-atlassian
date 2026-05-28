// Package priority implements the atlassian_jira_priority read-only data source.
//
// This data source reads Jira priorities by ID from the Atlassian Cloud REST API.
package priority

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

// apiPriority represents the JSON structure returned by the Atlassian priority API.
type apiPriority struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"iconUrl"`
	Self        string `json:"self"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	IconURL     types.String `tfsdk:"icon_url"`
}

// DataSource implements the atlassian_jira_priority data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_priority"
}

// Schema defines the schema for the jira priority data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira priority from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the priority.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the priority.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the priority.",
				Computed:    true,
			},
			"icon_url": schema.StringAttribute{
				Description: "The URL of the icon for the priority.",
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

// Read retrieves priority data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var p apiPriority
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/priority/%s", identifier), &p)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Priority not found",
				fmt.Sprintf("Jira priority %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read priority",
			fmt.Sprintf("Could not read Jira priority %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(p.ID)
	config.Name = types.StringValue(p.Name)
	config.Description = types.StringValue(p.Description)
	config.IconURL = types.StringValue(p.IconURL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
