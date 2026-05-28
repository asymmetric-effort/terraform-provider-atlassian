// Package workflow implements the atlassian_jira_workflow_scheme read-only data source.
//
// This data source reads Jira workflow schemes by ID from the Atlassian Cloud REST API.
package workflow

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the SchemeDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &SchemeDataSource{}

// apiWorkflowScheme represents the JSON structure returned by the Atlassian workflow scheme API.
type apiWorkflowScheme struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	DefaultWorkflowID string `json:"defaultWorkflow,omitempty"`
	Self              string `json:"self"`
}

// SchemeDataSourceModel describes the workflow scheme data source data model.
type SchemeDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	DefaultWorkflowID types.String `tfsdk:"default_workflow_id"`
}

// SchemeDataSource implements the atlassian_jira_workflow_scheme data source.
type SchemeDataSource struct {
	client *atlassian.Client
}

// NewSchemeDataSource returns a new SchemeDataSource instance for provider registration.
func NewSchemeDataSource() datasource.DataSource {
	return &SchemeDataSource{}
}

// Metadata returns the data source type name.
func (d *SchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow_scheme"
}

// Schema defines the schema for the jira workflow scheme data source.
func (d *SchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira workflow scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the workflow scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the workflow scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the workflow scheme.",
				Computed:    true,
			},
			"default_workflow_id": schema.StringAttribute{
				Description: "The ID of the default workflow for this scheme.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *SchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves workflow scheme data from the Atlassian API.
func (d *SchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var ws apiWorkflowScheme
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/workflowscheme/%s", identifier), &ws)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Workflow scheme not found",
				fmt.Sprintf("Jira workflow scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read workflow scheme",
			fmt.Sprintf("Could not read Jira workflow scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(ws.ID)
	config.Name = types.StringValue(ws.Name)
	config.Description = types.StringValue(ws.Description)
	config.DefaultWorkflowID = types.StringValue(ws.DefaultWorkflowID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
