// Package subscriber implements the atlassian_statuspage_subscriber read-only data source.
package subscriber

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

// apiSubscriber represents the JSON structure returned by the subscriber API.
type apiSubscriber struct {
	ID           string   `json:"id"`
	PageID       string   `json:"page_id"`
	Email        string   `json:"email"`
	Endpoint     string   `json:"endpoint"`
	ComponentIDs []string `json:"component_ids"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	PageID       types.String `tfsdk:"page_id"`
	Email        types.String `tfsdk:"email"`
	Endpoint     types.String `tfsdk:"endpoint"`
	ComponentIDs types.List   `tfsdk:"component_ids"`
}

// DataSource implements the atlassian_statuspage_subscriber data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_subscriber"
}

// Schema defines the schema for the subscriber data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Statuspage subscriber by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the subscriber.",
				Required:    true,
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
				Required:    true,
			},
			"email": schema.StringAttribute{
				Description: "The email address of the subscriber.",
				Computed:    true,
			},
			"endpoint": schema.StringAttribute{
				Description: "The webhook endpoint URL.",
				Computed:    true,
			},
			"component_ids": schema.ListAttribute{
				Description: "The list of component IDs subscribed to.",
				Computed:    true,
				ElementType: types.StringType,
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

// Read retrieves subscriber data from the API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var sub apiSubscriber
	err := d.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/subscribers/%s", config.PageID.ValueString(), config.ID.ValueString()), &sub)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Subscriber not found",
				fmt.Sprintf("Subscriber %q on page %q not found.", config.ID.ValueString(), config.PageID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage subscriber",
			fmt.Sprintf("Could not read subscriber %q on page %q: %s", config.ID.ValueString(), config.PageID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(sub.ID)
	config.PageID = types.StringValue(sub.PageID)
	config.Email = types.StringValue(sub.Email)
	config.Endpoint = types.StringValue(sub.Endpoint)
	compIDs, diags := types.ListValueFrom(ctx, types.StringType, sub.ComponentIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	config.ComponentIDs = compIDs

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
