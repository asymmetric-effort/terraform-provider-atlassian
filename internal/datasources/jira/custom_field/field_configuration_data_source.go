// Package customfield implements the atlassian_jira_field_configuration read-only data source.
//
// This data source reads Jira field configurations by ID from the Atlassian Cloud REST API.
package customfield

import (
	"context"
	"fmt"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// idToString converts a JSON id field (string or number) to a string.
func idToString(v interface{}) string {
	return fmt.Sprintf("%v", v)
}

// Ensure the FieldConfigurationDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &FieldConfigurationDataSource{}

// apiFieldConfigurationDS represents the JSON structure returned by the Atlassian field configuration API.
type apiFieldConfigurationDS struct {
	ID          interface{} `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Self        string      `json:"self"`
}

// apiFieldConfigurationDSList represents the paginated list response.
type apiFieldConfigurationDSList struct {
	Values []apiFieldConfigurationDS `json:"values"`
}

// FieldConfigurationDataSourceModel describes the field configuration data source data model.
type FieldConfigurationDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// FieldConfigurationDataSource implements the atlassian_jira_field_configuration data source.
type FieldConfigurationDataSource struct {
	client *atlassian.Client
}

// NewFieldConfigurationDataSource returns a new FieldConfigurationDataSource instance for provider registration.
func NewFieldConfigurationDataSource() datasource.DataSource {
	return &FieldConfigurationDataSource{}
}

// Metadata returns the data source type name.
func (d *FieldConfigurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_field_configuration"
}

// Schema defines the schema for the jira field configuration data source.
func (d *FieldConfigurationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira field configuration from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the field configuration.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the field configuration.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the field configuration.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *FieldConfigurationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves field configuration data from the Atlassian API.
func (d *FieldConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FieldConfigurationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var list apiFieldConfigurationDSList
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/fieldconfiguration?id=%s", identifier), &list)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read field configuration",
			fmt.Sprintf("Could not read Jira field configuration %q: %s", identifier, err.Error()),
		)
		return
	}

	if len(list.Values) == 0 {
		resp.Diagnostics.AddError(
			"Field configuration not found",
			fmt.Sprintf("Jira field configuration %q not found. Verify the ID is correct.", identifier),
		)
		return
	}

	fc := list.Values[0]
	config.ID = types.StringValue(idToString(fc.ID))
	config.Name = types.StringValue(fc.Name)
	config.Description = types.StringValue(fc.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
