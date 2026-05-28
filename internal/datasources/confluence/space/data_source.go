// Package space implements the atlassian_confluence_space read-only data source.
//
// This data source reads Confluence spaces by ID or key from the
// Atlassian Cloud REST API (v2).
package space

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiSpaceDescription wraps the description object in the API response.
type apiSpaceDescription struct {
	Plain struct {
		Value string `json:"value"`
	} `json:"plain"`
}

// apiSpaceLinks holds link information from the API response.
type apiSpaceLinks struct {
	WebUI string `json:"webui"`
}

// apiSpaceFullResponse represents the full API response with nested description.
type apiSpaceFullResponse struct {
	ID          string              `json:"id"`
	Key         string              `json:"key"`
	Name        string              `json:"name"`
	Description apiSpaceDescription `json:"description"`
	Type        string              `json:"type"`
	HomepageID  string              `json:"homepageId"`
	Status      string              `json:"status"`
	Links       apiSpaceLinks       `json:"_links"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Key         types.String `tfsdk:"key"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Type        types.String `tfsdk:"type"`
	HomepageID  types.String `tfsdk:"homepage_id"`
	Status      types.String `tfsdk:"status"`
	URL         types.String `tfsdk:"url"`
}

// DataSource implements the atlassian_confluence_space data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_space"
}

// Schema defines the schema for the Confluence space data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Confluence space from Atlassian Cloud by ID or key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the space. Exactly one of id or key must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"key": schema.StringAttribute{
				Description: "The space key (e.g., ENG). Exactly one of id or key must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the space.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A plain-text description of the space.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of space (\"global\" or \"personal\").",
				Computed:    true,
			},
			"homepage_id": schema.StringAttribute{
				Description: "The ID of the space's homepage.",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The status of the space (e.g., current, archived).",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the space in Atlassian Cloud.",
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

// parseSpaceResponse parses a raw JSON response into data source model fields.
func parseSpaceResponse(raw json.RawMessage) (id, key, name, description, spaceType, homepageID, status, url string) {
	var resp apiSpaceFullResponse
	if json.Unmarshal(raw, &resp) == nil {
		id = resp.ID
		key = resp.Key
		name = resp.Name
		description = resp.Description.Plain.Value
		spaceType = resp.Type
		homepageID = resp.HomepageID
		status = resp.Status
		url = resp.Links.WebUI
	}
	return
}

// Read retrieves Confluence space data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()
	hasKey := !config.Key.IsNull() && !config.Key.IsUnknown()

	if !hasID && !hasKey {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"Exactly one of id or key must be specified to look up a Confluence space.",
		)
		return
	}

	identifier := config.ID.ValueString()
	if !hasID {
		identifier = config.Key.ValueString()
	}

	var raw json.RawMessage
	err := d.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", identifier), &raw)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Confluence space not found",
				fmt.Sprintf("Confluence space %q not found. Verify the ID or key is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Confluence space",
			fmt.Sprintf("Could not read Confluence space %q: %s", identifier, err.Error()),
		)
		return
	}

	id, key, name, description, spaceType, homepageID, status, url := parseSpaceResponse(raw)
	config.ID = types.StringValue(id)
	config.Key = types.StringValue(key)
	config.Name = types.StringValue(name)
	config.Description = types.StringValue(description)
	config.Type = types.StringValue(spaceType)
	config.HomepageID = types.StringValue(homepageID)
	config.Status = types.StringValue(status)
	config.URL = types.StringValue(url)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
