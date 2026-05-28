// Package component implements the atlassian_statuspage_component_group read-only data source.
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

// Ensure the GroupDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &GroupDataSource{}

// apiComponentGroup represents the JSON structure returned by the component group API.
type apiComponentGroup struct {
	ID          string `json:"id"`
	PageID      string `json:"page_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GroupDataSourceModel describes the data source data model.
type GroupDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	PageID      types.String `tfsdk:"page_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// GroupDataSource implements the data source.
type GroupDataSource struct {
	client *atlassian.Client
}

// NewGroupDataSource returns a new instance for provider registration.
func NewGroupDataSource() datasource.DataSource {
	return &GroupDataSource{}
}

// Metadata returns the data source type name.
func (d *GroupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_component_group"
}

// Schema defines the schema for the data source.
func (d *GroupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Statuspage component group by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the component group.",
				Required:    true,
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the component group.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the component group.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client.
func (d *GroupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *GroupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config GroupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var group apiComponentGroup
	err := d.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/component-groups/%s", config.PageID.ValueString(), config.ID.ValueString()), &group)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Component group not found",
				fmt.Sprintf("Component group %q on page %q not found.", config.ID.ValueString(), config.PageID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage component group",
			fmt.Sprintf("Could not read component group %q on page %q: %s", config.ID.ValueString(), config.PageID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(group.ID)
	config.PageID = types.StringValue(group.PageID)
	config.Name = types.StringValue(group.Name)
	config.Description = types.StringValue(group.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
