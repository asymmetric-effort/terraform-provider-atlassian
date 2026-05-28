// Package page implements the atlassian_statuspage_maintenance_template read-only data source.
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

// Ensure the MaintenanceTemplateDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &MaintenanceTemplateDataSource{}

// apiMaintenanceTemplate represents the JSON structure for maintenance template API responses.
type apiMaintenanceTemplate struct {
	ID     string `json:"id"`
	PageID string `json:"page_id"`
	Name   string `json:"name"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// MaintenanceTemplateDataSourceModel describes the data source data model.
type MaintenanceTemplateDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	PageID types.String `tfsdk:"page_id"`
	Name   types.String `tfsdk:"name"`
	Title  types.String `tfsdk:"title"`
	Body   types.String `tfsdk:"body"`
}

// MaintenanceTemplateDataSource implements the data source.
type MaintenanceTemplateDataSource struct {
	client *atlassian.Client
}

// NewMaintenanceTemplateDataSource returns a new instance for provider registration.
func NewMaintenanceTemplateDataSource() datasource.DataSource {
	return &MaintenanceTemplateDataSource{}
}

// Metadata returns the data source type name.
func (d *MaintenanceTemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_maintenance_template"
}

// Schema defines the schema for the data source.
func (d *MaintenanceTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Statuspage maintenance template by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the maintenance template.",
				Required:    true,
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the maintenance template.",
				Computed:    true,
			},
			"title": schema.StringAttribute{
				Description: "The default title for maintenance events.",
				Computed:    true,
			},
			"body": schema.StringAttribute{
				Description: "The default body for maintenance events.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client.
func (d *MaintenanceTemplateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves data from the API.
func (d *MaintenanceTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config MaintenanceTemplateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tmpl apiMaintenanceTemplate
	err := d.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", config.PageID.ValueString(), config.ID.ValueString()), &tmpl)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Maintenance template not found",
				fmt.Sprintf("Maintenance template %q on page %q not found.", config.ID.ValueString(), config.PageID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read maintenance template",
			fmt.Sprintf("Could not read maintenance template %q on page %q: %s", config.ID.ValueString(), config.PageID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(tmpl.ID)
	config.PageID = types.StringValue(tmpl.PageID)
	config.Name = types.StringValue(tmpl.Name)
	config.Title = types.StringValue(tmpl.Title)
	config.Body = types.StringValue(tmpl.Body)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
