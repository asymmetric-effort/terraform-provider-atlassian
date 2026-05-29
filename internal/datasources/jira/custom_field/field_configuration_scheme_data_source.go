// Package customfield implements the atlassian_jira_field_configuration_scheme read-only data source.
//
// This data source reads Jira field configuration schemes by ID from the Atlassian Cloud REST API.
package customfield

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the FieldConfigurationSchemeDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &FieldConfigurationSchemeDataSource{}

// apiFieldConfigurationSchemeDS represents the JSON structure returned by the Atlassian field configuration scheme API.
type apiFieldConfigurationSchemeDS struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Self        string `json:"self"`
}

// FieldConfigurationSchemeDataSourceModel describes the field configuration scheme data source data model.
type FieldConfigurationSchemeDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// FieldConfigurationSchemeDataSource implements the atlassian_jira_field_configuration_scheme data source.
type FieldConfigurationSchemeDataSource struct {
	client *atlassian.Client
}

// NewFieldConfigurationSchemeDataSource returns a new FieldConfigurationSchemeDataSource instance for provider registration.
func NewFieldConfigurationSchemeDataSource() datasource.DataSource {
	return &FieldConfigurationSchemeDataSource{}
}

// Metadata returns the data source type name.
func (d *FieldConfigurationSchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_field_configuration_scheme"
}

// Schema defines the schema for the jira field configuration scheme data source.
func (d *FieldConfigurationSchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira field configuration scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the field configuration scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the field configuration scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the field configuration scheme.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *FieldConfigurationSchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves field configuration scheme data from the Atlassian API.
func (d *FieldConfigurationSchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FieldConfigurationSchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var fcs apiFieldConfigurationSchemeDS
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/fieldconfigurationscheme/%s", identifier), &fcs)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Field configuration scheme not found",
				fmt.Sprintf("Jira field configuration scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read field configuration scheme",
			fmt.Sprintf("Could not read Jira field configuration scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(fcs.ID)
	config.Name = types.StringValue(fcs.Name)
	config.Description = types.StringValue(fcs.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
