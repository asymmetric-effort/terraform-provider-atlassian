// Package page implements the atlassian_statuspage_permission read-only data source.
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

// Ensure the PermissionDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &PermissionDataSource{}

// apiPermissionSP represents the JSON structure for Statuspage permissions.
type apiPermissionSP struct {
	ID            string `json:"id"`
	PageID        string `json:"page_id"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Role          string `json:"role"`
}

// PermissionDataSourceModel describes the data source data model.
type PermissionDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	PageID        types.String `tfsdk:"page_id"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Role          types.String `tfsdk:"role"`
}

// PermissionDataSource implements the data source.
type PermissionDataSource struct {
	client *atlassian.Client
}

// NewPermissionDataSource returns a new instance for provider registration.
func NewPermissionDataSource() datasource.DataSource {
	return &PermissionDataSource{}
}

// Metadata returns the data source type name.
func (d *PermissionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_permission"
}

// Schema defines the schema for the data source.
func (d *PermissionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Statuspage page permission by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the permission.",
				Required:    true,
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
				Required:    true,
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal (\"user\" or \"group\").",
				Computed:    true,
			},
			"principal_id": schema.StringAttribute{
				Description: "The ID of the principal.",
				Computed:    true,
			},
			"role": schema.StringAttribute{
				Description: "The role assigned (\"admin\", \"member\", or \"viewer\").",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client.
func (d *PermissionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
func (d *PermissionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PermissionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var perm apiPermissionSP
	err := d.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/permissions/%s", config.PageID.ValueString(), config.ID.ValueString()), &perm)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Permission not found",
				fmt.Sprintf("Permission %q on page %q not found.", config.ID.ValueString(), config.PageID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage permission",
			fmt.Sprintf("Could not read permission %q on page %q: %s", config.ID.ValueString(), config.PageID.ValueString(), err.Error()),
		)
		return
	}

	config.ID = types.StringValue(perm.ID)
	config.PageID = types.StringValue(perm.PageID)
	config.PrincipalType = types.StringValue(perm.PrincipalType)
	config.PrincipalID = types.StringValue(perm.PrincipalID)
	config.Role = types.StringValue(perm.Role)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
