// Package page implements the atlassian_statuspage_incident_template read-only data source.
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

// Ensure the IncidentTemplateDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &IncidentTemplateDataSource{}

// apiIncidentTemplate represents the JSON structure for incident template API responses.
type apiIncidentTemplate struct {
	ID     string `json:"id"`
	PageID string `json:"page_id"`
	Name   string `json:"name"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// IncidentTemplateDataSourceModel describes the data source data model.
type IncidentTemplateDataSourceModel struct {
	ID     types.String `tfsdk:"id"`
	PageID types.String `tfsdk:"page_id"`
	Name   types.String `tfsdk:"name"`
	Title  types.String `tfsdk:"title"`
	Body   types.String `tfsdk:"body"`
}

// IncidentTemplateDataSource implements the data source.
type IncidentTemplateDataSource struct {
	client *atlassian.Client
}

// NewIncidentTemplateDataSource returns a new instance for provider registration.
func NewIncidentTemplateDataSource() datasource.DataSource {
	return &IncidentTemplateDataSource{}
}

// Metadata returns the data source type name.
func (d *IncidentTemplateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_incident_template"
}

// Schema defines the schema for the data source.
func (d *IncidentTemplateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Statuspage incident template by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the incident template.",
				Required:    true,
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the incident template.",
				Computed:    true,
			},
			"title": schema.StringAttribute{
				Description: "The default title for incidents.",
				Computed:    true,
			},
			"body": schema.StringAttribute{
				Description: "The default body for incidents.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client.
func (d *IncidentTemplateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *IncidentTemplateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config IncidentTemplateDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tmpl apiIncidentTemplate
	err := d.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates/%s", config.PageID.ValueString(), config.ID.ValueString()), &tmpl)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Incident template not found",
				fmt.Sprintf("Incident template %q on page %q not found.", config.ID.ValueString(), config.PageID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read incident template",
			fmt.Sprintf("Could not read incident template %q on page %q: %s", config.ID.ValueString(), config.PageID.ValueString(), err.Error()),
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
