// Package template implements the atlassian_confluence_template read-only data source.
//
// This data source reads Confluence templates by ID from the Atlassian Cloud REST API (v2).
package template

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

// apiTemplate represents the JSON structure returned by the Confluence template API.
type apiTemplate struct {
	TemplateID   string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Body         string `json:"body,omitempty"`
	SpaceID      string `json:"spaceId,omitempty"`
	TemplateType string `json:"templateType,omitempty"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Body         types.String `tfsdk:"body"`
	SpaceID      types.String `tfsdk:"space_id"`
	TemplateType types.String `tfsdk:"template_type"`
}

// DataSource implements the atlassian_confluence_template data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_template"
}

// Schema defines the schema for the confluence template data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Confluence template from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the template.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the template.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the template.",
				Computed:    true,
			},
			"body": schema.StringAttribute{
				Description: "The body content of the template.",
				Computed:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the space for a space-scoped template.",
				Computed:    true,
			},
			"template_type": schema.StringAttribute{
				Description: "The type of template.",
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

// Read retrieves template data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	templateID := config.ID.ValueString()

	var tmpl apiTemplate
	err := d.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", templateID), &tmpl)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Template not found",
				fmt.Sprintf("Confluence template %q not found.", templateID),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read template",
			fmt.Sprintf("Could not read Confluence template %q: %s", templateID, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(tmpl.TemplateID)
	config.Name = types.StringValue(tmpl.Name)
	config.Description = types.StringValue(tmpl.Description)
	config.Body = types.StringValue(tmpl.Body)
	config.SpaceID = types.StringValue(tmpl.SpaceID)
	config.TemplateType = types.StringValue(tmpl.TemplateType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
