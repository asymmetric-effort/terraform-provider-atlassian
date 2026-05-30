// Package organization implements the atlassian_organization read-only data source.
//
// This data source reads an Atlassian Cloud organization by ID from the Admin API.
package organization

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

// apiOrganizationDSResponse represents the JSON envelope from the Admin API.
type apiOrganizationDSResponse struct {
	Data apiOrganizationDS `json:"data"`
}

// apiOrganizationDS represents the JSON structure from the Atlassian organization API.
type apiOrganizationDS struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Name string `json:"name"`
	} `json:"attributes"`
}

// DataSourceModel describes the organization data source data model.
type DataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

// DataSource implements the atlassian_organization data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

// Schema defines the schema for the organization data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an Atlassian Cloud organization by ID from the Admin API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the organization.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the organization.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of the organization.",
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

// Read retrieves organization data from the Atlassian Admin API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiResp apiOrganizationDSResponse
	err := d.client.AdminGet(ctx, fmt.Sprintf("/v1/orgs/%s", config.ID.ValueString()), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Organization not found",
				fmt.Sprintf("Atlassian organization %q not found. Verify the ID is correct.", config.ID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read organization",
			fmt.Sprintf("Could not read Atlassian organization %q: %s", config.ID.ValueString(), err.Error()),
		)
		return
	}

	org := apiResp.Data
	config.ID = types.StringValue(org.ID)
	config.Name = types.StringValue(org.Attributes.Name)
	config.Type = types.StringValue(org.Type)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
