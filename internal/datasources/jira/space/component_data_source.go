// Package space implements Jira space (project) data sources including
// project components.
//
// ComponentDataSource reads a Jira project component by ID from the
// Atlassian Cloud REST API (/rest/api/3/component).
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

// Ensure ComponentDataSource satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &ComponentDataSource{}

// apiComponentDS represents the JSON structure returned by the Atlassian component API.
type apiComponentDS struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	LeadAccountID string `json:"leadAccountId,omitempty"`
	AssigneeType  string `json:"assigneeType,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
	Self          string `json:"self,omitempty"`
}

// ComponentDataSourceModel describes the component data source data model.
type ComponentDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	SpaceID       types.String `tfsdk:"space_id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	LeadAccountID types.String `tfsdk:"lead_account_id"`
	AssigneeType  types.String `tfsdk:"assignee_type"`
}

// ComponentDataSource implements the atlassian_jira_project_component data source.
type ComponentDataSource struct {
	client *atlassian.Client
}

// NewComponentDataSource returns a new ComponentDataSource instance for provider registration.
func NewComponentDataSource() datasource.DataSource {
	return &ComponentDataSource{}
}

// Metadata returns the data source type name.
func (d *ComponentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_project_component"
}

// Schema defines the schema for the jira project component data source.
func (d *ComponentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira project component from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the component.",
				Required:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The project ID that the component belongs to.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the component.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the component.",
				Computed:    true,
			},
			"lead_account_id": schema.StringAttribute{
				Description: "The account ID of the component lead.",
				Computed:    true,
			},
			"assignee_type": schema.StringAttribute{
				Description: "The assignee type for issues in this component.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *ComponentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves component data from the Atlassian API.
func (d *ComponentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config ComponentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	componentID := config.ID.ValueString()
	if componentID == "" {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"The id attribute must be specified to look up a Jira project component.",
		)
		return
	}

	var component apiComponentDS
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/component/%s", componentID), &component)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Component not found",
				fmt.Sprintf("Jira project component %q not found. Verify the ID is correct.", componentID),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read component",
			fmt.Sprintf("Could not read Jira project component %q: %s", componentID, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(component.ID)
	config.SpaceID = types.StringValue(component.ProjectID)
	config.Name = types.StringValue(component.Name)
	config.Description = types.StringValue(component.Description)
	config.LeadAccountID = types.StringValue(component.LeadAccountID)
	config.AssigneeType = types.StringValue(component.AssigneeType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
