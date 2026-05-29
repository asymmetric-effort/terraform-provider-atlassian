// Package issuelink implements the atlassian_jira_issue_link_type read-only data source.
//
// This data source reads Jira issue link types by ID from the Atlassian Cloud REST API.
package issuelink

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the TypeDataSource satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &TypeDataSource{}

// apiIssueLinkTypeDS represents the JSON structure returned by the Atlassian issue link type API.
type apiIssueLinkTypeDS struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
	Self    string `json:"self"`
}

// TypeDataSourceModel describes the issue link type data source data model.
type TypeDataSourceModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Inward  types.String `tfsdk:"inward"`
	Outward types.String `tfsdk:"outward"`
}

// TypeDataSource implements the atlassian_jira_issue_link_type data source.
type TypeDataSource struct {
	client *atlassian.Client
}

// NewTypeDataSource returns a new TypeDataSource instance for provider registration.
func NewTypeDataSource() datasource.DataSource {
	return &TypeDataSource{}
}

// Metadata returns the data source type name.
func (d *TypeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_link_type"
}

// Schema defines the schema for the jira issue link type data source.
func (d *TypeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira issue link type from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the issue link type.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the issue link type.",
				Computed:    true,
			},
			"inward": schema.StringAttribute{
				Description: "The inward description of the link.",
				Computed:    true,
			},
			"outward": schema.StringAttribute{
				Description: "The outward description of the link.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *TypeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves issue link type data from the Atlassian API.
func (d *TypeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TypeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var ilt apiIssueLinkTypeDS
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/issueLinkType/%s", identifier), &ilt)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Issue link type not found",
				fmt.Sprintf("Jira issue link type %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read issue link type",
			fmt.Sprintf("Could not read Jira issue link type %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(ilt.ID)
	config.Name = types.StringValue(ilt.Name)
	config.Inward = types.StringValue(ilt.Inward)
	config.Outward = types.StringValue(ilt.Outward)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
