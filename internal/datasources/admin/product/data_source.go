// Package product implements the atlassian_product read-only data source.
//
// This data source queries workspaces within an Atlassian organization to find
// provisioned product instances.
package product

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiWorkspaceQueryDS represents the workspace query request.
type apiWorkspaceQueryDS struct {
	Query struct {
		Field struct {
			Name   string   `json:"name"`
			Values []string `json:"values"`
		} `json:"field"`
	} `json:"query"`
}

// apiWorkspaceResponseDS represents the workspace query response.
type apiWorkspaceResponseDS struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"attributes"`
	} `json:"data"`
}

// DataSourceModel describes the product data source data model.
type DataSourceModel struct {
	OrgID    types.String `tfsdk:"org_id"`
	SiteName types.String `tfsdk:"site_name"`
	ID       types.String `tfsdk:"id"`
	SiteURL  types.String `tfsdk:"site_url"`
}

// DataSource implements the atlassian_product data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

// Schema defines the schema for the product data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an Atlassian Cloud product instance (workspace) by organization ID and site name.",
		Attributes: map[string]schema.Attribute{
			"org_id": schema.StringAttribute{
				Description: "The ID of the Atlassian organization.",
				Required:    true,
			},
			"site_name": schema.StringAttribute{
				Description: "The site name to look up.",
				Required:    true,
			},
			"id": schema.StringAttribute{
				Description: "The workspace ID of the product instance.",
				Computed:    true,
			},
			"site_url": schema.StringAttribute{
				Description: "The URL of the product instance.",
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

// Read queries workspaces to find a product instance.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID := config.OrgID.ValueString()
	siteName := config.SiteName.ValueString()

	query := apiWorkspaceQueryDS{}
	query.Query.Field.Name = "attributes.name"
	query.Query.Field.Values = []string{siteName}
	bodyBytes, _ := json.Marshal(query)

	var wsResp apiWorkspaceResponseDS
	err := d.client.AdminPost(ctx,
		fmt.Sprintf("/v2/orgs/%s/workspaces", orgID),
		bytes.NewReader(bodyBytes), &wsResp)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to query workspaces",
			fmt.Sprintf("Could not query workspaces in organization %q: %s", orgID, err.Error()),
		)
		return
	}

	for _, ws := range wsResp.Data {
		if ws.Attributes.Name == siteName {
			config.ID = types.StringValue(ws.ID)
			config.SiteURL = types.StringValue(ws.Attributes.URL)
			resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
			return
		}
	}

	resp.Diagnostics.AddError(
		"Product instance not found",
		fmt.Sprintf("No workspace named %q found in organization %q.", siteName, orgID),
	)
}
