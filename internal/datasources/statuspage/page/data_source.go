// Package page implements the atlassian_statuspage_page read-only data source.
package page

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

// apiPage represents the JSON structure returned by the Statuspage page API.
type apiPage struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	PageDescription string `json:"page_description"`
	Subdomain       string `json:"subdomain"`
	URL             string `json:"url"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	PageDescription types.String `tfsdk:"page_description"`
	Subdomain       types.String `tfsdk:"subdomain"`
	URL             types.String `tfsdk:"url"`
}

// DataSource implements the atlassian_statuspage_page data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_page"
}

// Schema defines the schema for the Statuspage page data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Statuspage page by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the page.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the Statuspage page.",
				Computed:    true,
			},
			"page_description": schema.StringAttribute{
				Description: "A description of the Statuspage page.",
				Computed:    true,
			},
			"subdomain": schema.StringAttribute{
				Description: "The subdomain for the Statuspage page.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the Statuspage page.",
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

// Read retrieves Statuspage page data from the API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var page apiPage
	err := d.client.Get(ctx, fmt.Sprintf("/v1/pages/%s", config.ID.ValueString()), &page)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Statuspage page not found",
				fmt.Sprintf("Statuspage page %q not found. Verify the ID is correct.", config.ID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage page",
			fmt.Sprintf("Could not read Statuspage page %q: %s", config.ID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(page.ID)
	config.Name = types.StringValue(page.Name)
	config.PageDescription = types.StringValue(page.PageDescription)
	config.Subdomain = types.StringValue(page.Subdomain)
	config.URL = types.StringValue(page.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
