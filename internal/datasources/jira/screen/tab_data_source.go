// Package screen implements the atlassian_jira_screen_tab read-only data source.
//
// This data source reads Jira screen tabs by ID from the Atlassian Cloud REST API.
package screen

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the TabDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &TabDataSource{}

// apiTabDS represents the JSON structure returned by the Atlassian screen tab API (data source).
type apiTabDS struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position,omitempty"`
}

// TabDataSourceModel describes the data source data model for screen tabs.
type TabDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	ScreenID types.String `tfsdk:"screen_id"`
	Name     types.String `tfsdk:"name"`
	Position types.Int64  `tfsdk:"position"`
}

// TabDataSource implements the atlassian_jira_screen_tab data source.
type TabDataSource struct {
	client *atlassian.Client
}

// NewTabDataSource returns a new TabDataSource instance for provider registration.
func NewTabDataSource() datasource.DataSource {
	return &TabDataSource{}
}

// Metadata returns the data source type name.
func (d *TabDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_tab"
}

// Schema defines the schema for the jira screen tab data source.
func (d *TabDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira screen tab from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the screen tab.",
				Required:    true,
			},
			"screen_id": schema.StringAttribute{
				Description: "The ID of the screen that owns this tab.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the screen tab.",
				Computed:    true,
			},
			"position": schema.Int64Attribute{
				Description: "The position of the tab on the screen, used for ordering.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *TabDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves screen tab data from the Atlassian API.
func (d *TabDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config TabDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	screenID := config.ScreenID.ValueString()
	tabID := config.ID.ValueString()

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs", screenID)

	var tabs []apiTabDS
	err := d.client.Get(ctx, endpoint, &tabs)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Screen not found",
				fmt.Sprintf("Jira screen %q not found. Verify the screen ID is correct.", screenID),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read screen tabs",
			fmt.Sprintf("Could not read tabs for Jira screen %q: %s", screenID, err.Error()),
		)
		return
	}

	// Find the specific tab in the list
	for _, tab := range tabs {
		if fmt.Sprintf("%d", tab.ID) == tabID {
			config.ID = types.StringValue(tabID)
			config.Name = types.StringValue(tab.Name)
			config.Position = types.Int64Value(int64(tab.Position))
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Screen tab not found",
		fmt.Sprintf("Jira screen tab %q not found on screen %q. Verify the tab ID is correct.", tabID, screenID),
	)
}
