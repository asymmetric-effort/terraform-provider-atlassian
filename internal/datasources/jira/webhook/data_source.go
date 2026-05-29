// Package webhook implements the atlassian_jira_webhook read-only data source.
//
// This data source reads Jira webhooks by ID from the Atlassian Cloud REST API.
package webhook

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

// apiWebhookDS represents the JSON structure returned by the Atlassian webhook API.
type apiWebhookDS struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	JQLFilter string   `json:"jqlFilter"`
	Enabled   bool     `json:"enabled"`
	Self      string   `json:"self"`
}

// DataSourceModel describes the webhook data source data model.
type DataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	URL       types.String `tfsdk:"url"`
	Events    types.List   `tfsdk:"events"`
	JQLFilter types.String `tfsdk:"jql_filter"`
	Enabled   types.Bool   `tfsdk:"enabled"`
}

// DataSource implements the atlassian_jira_webhook data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_webhook"
}

// Schema defines the schema for the jira webhook data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira webhook from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the webhook.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the webhook.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL that the webhook posts event data to.",
				Computed:    true,
			},
			"events": schema.ListAttribute{
				Description: "The list of Jira event types the webhook listens for.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"jql_filter": schema.StringAttribute{
				Description: "The JQL filter that restricts which issues trigger the webhook.",
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the webhook is enabled.",
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

// Read retrieves webhook data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var wh apiWebhookDS
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/webhook/%s", identifier), &wh)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Webhook not found",
				fmt.Sprintf("Jira webhook %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read webhook",
			fmt.Sprintf("Could not read Jira webhook %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(wh.ID)
	config.Name = types.StringValue(wh.Name)
	config.URL = types.StringValue(wh.URL)

	eventsList, _ := types.ListValueFrom(ctx, types.StringType, wh.Events)
	config.Events = eventsList
	config.JQLFilter = types.StringValue(wh.JQLFilter)
	config.Enabled = types.BoolValue(wh.Enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
