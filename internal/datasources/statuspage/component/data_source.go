// Package component implements the atlassian_statuspage_component read-only data source.
package component

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

// apiComponent represents the JSON structure returned by the component API.
type apiComponent struct {
	ID          string `json:"id"`
	PageID      string `json:"page_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	GroupID     string `json:"group_id"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	PageID      types.String `tfsdk:"page_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Status      types.String `tfsdk:"status"`
	GroupID     types.String `tfsdk:"group_id"`
}

// DataSource implements the atlassian_statuspage_component data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_component"
}

// Schema defines the schema for the component data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Statuspage component by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the component.",
				Required:    true,
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the component.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the component.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The status of the component.",
				Computed:    true,
			},
			"group_id": schema.StringAttribute{
				Description: "The ID of the component group.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client.
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

// Read retrieves component data from the API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var comp apiComponent
	err := d.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/components/%s", config.PageID.ValueString(), config.ID.ValueString()), &comp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Component not found",
				fmt.Sprintf("Component %q on page %q not found.", config.ID.ValueString(), config.PageID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage component",
			fmt.Sprintf("Could not read component %q on page %q: %s", config.ID.ValueString(), config.PageID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(comp.ID)
	config.PageID = types.StringValue(comp.PageID)
	config.Name = types.StringValue(comp.Name)
	config.Description = types.StringValue(comp.Description)
	config.Status = types.StringValue(comp.Status)
	config.GroupID = types.StringValue(comp.GroupID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
