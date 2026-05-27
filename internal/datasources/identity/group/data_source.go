// Package group implements the atlassian_group data source.
//
// This data source reads an existing Atlassian Cloud group by ID or name.
package group

import (
	"context"
	"fmt"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure DataSource satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// DataSource implements the atlassian_group data source.
type DataSource struct {
	client *atlassian.Client
}

// DataSourceModel describes the data source data model for an Atlassian group.
type DataSourceModel struct {
	GroupID types.String `tfsdk:"group_id"`
	Name    types.String `tfsdk:"name"`
	SelfURL types.String `tfsdk:"self_url"`
}

// apiGroupResponse represents the JSON response from the Atlassian group API.
type apiGroupResponse struct {
	GroupID string `json:"groupId"`
	Name    string `json:"name"`
	Self    string `json:"self"`
}

// NewDataSource returns a new instance of the group data source.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

// Schema defines the schema for the group data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Atlassian Cloud group by ID or name.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				Description: "The unique identifier of the group. Provide either group_id or name.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the group. Provide either group_id or name.",
				Optional:    true,
				Computed:    true,
			},
			"self_url": schema.StringAttribute{
				Description: "The URL of the group in the Atlassian REST API.",
				Computed:    true,
			},
		},
	}
}

// Configure retrieves the provider-configured client for API calls.
func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DataSource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = client
}

// Read retrieves an Atlassian group by ID or name.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := config.GroupID.ValueString()
	groupName := config.Name.ValueString()

	if groupID == "" && groupName == "" {
		resp.Diagnostics.AddError(
			"Missing required argument",
			"Either 'group_id' or 'name' must be specified to look up a group.",
		)
		return
	}

	var apiResp apiGroupResponse
	var apiPath string

	if groupID != "" {
		apiPath = fmt.Sprintf("/rest/api/3/group?groupId=%s", groupID)
	} else {
		apiPath = fmt.Sprintf("/rest/api/3/group?groupname=%s", groupName)
	}

	err := d.client.Get(ctx, apiPath, &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case 404:
				resp.Diagnostics.AddError(
					"Group not found",
					fmt.Sprintf("No group found with the specified identifier. Verify the group exists and the identifier is correct."),
				)
				return
			case 403:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to read this group. "+
						"Ensure the user has the 'Browse users and groups' global permission.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to read group",
			"Could not read group: "+err.Error(),
		)
		return
	}

	config.GroupID = types.StringValue(apiResp.GroupID)
	config.Name = types.StringValue(apiResp.Name)
	config.SelfURL = types.StringValue(apiResp.Self)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
