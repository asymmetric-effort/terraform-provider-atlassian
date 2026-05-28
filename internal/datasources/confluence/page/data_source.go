// Package page implements the atlassian_confluence_page read-only data source.
//
// This data source reads Confluence pages by ID from the Atlassian Cloud REST API (v2).
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

// apiPage represents the JSON structure returned by the Confluence page API.
type apiPage struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	SpaceID  string         `json:"spaceId"`
	ParentID string         `json:"parentId,omitempty"`
	Status   string         `json:"status"`
	Body     apiPageBody    `json:"body,omitempty"`
	Version  apiPageVersion `json:"version,omitempty"`
}

// apiPageBody represents the body content of a Confluence page.
type apiPageBody struct {
	Storage apiPageStorage `json:"storage,omitempty"`
}

// apiPageStorage represents the storage-format body content.
type apiPageStorage struct {
	Value          string `json:"value,omitempty"`
	Representation string `json:"representation,omitempty"`
}

// apiPageVersion represents the version information of a page.
type apiPageVersion struct {
	Number int64 `json:"number"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	SpaceID  types.String `tfsdk:"space_id"`
	Title    types.String `tfsdk:"title"`
	Body     types.String `tfsdk:"body"`
	ParentID types.String `tfsdk:"parent_id"`
	Status   types.String `tfsdk:"status"`
	Version  types.Int64  `tfsdk:"version"`
}

// DataSource implements the atlassian_confluence_page data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_page"
}

// Schema defines the schema for the confluence page data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Confluence page from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the page.",
				Required:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the space containing the page.",
				Computed:    true,
			},
			"title": schema.StringAttribute{
				Description: "The title of the page.",
				Computed:    true,
			},
			"body": schema.StringAttribute{
				Description: "The body content of the page in storage format.",
				Computed:    true,
			},
			"parent_id": schema.StringAttribute{
				Description: "The ID of the parent page.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The status of the page.",
				Computed:    true,
			},
			"version": schema.Int64Attribute{
				Description: "The current version number of the page.",
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

// Read retrieves page data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pageID := config.ID.ValueString()

	var page apiPage
	err := d.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s?body-format=storage", pageID), &page)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Page not found",
				fmt.Sprintf("Confluence page %q not found.", pageID),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read page",
			fmt.Sprintf("Could not read Confluence page %q: %s", pageID, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(page.ID)
	config.SpaceID = types.StringValue(page.SpaceID)
	config.Title = types.StringValue(page.Title)
	config.Body = types.StringValue(page.Body.Storage.Value)
	config.ParentID = types.StringValue(page.ParentID)
	config.Status = types.StringValue(page.Status)
	config.Version = types.Int64Value(page.Version.Number)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
