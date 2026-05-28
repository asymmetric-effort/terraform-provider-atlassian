// Package space implements the atlassian_confluence_space_permission read-only data source.
//
// This data source reads space permissions from the Atlassian Cloud REST API (v2).
package space

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

// apiPermission represents the JSON structure returned by the space permission API.
type apiPermission struct {
	ID        string             `json:"id"`
	Principal apiPermissionPrinc `json:"principal"`
	Operation apiPermissionOp    `json:"operation"`
}

// apiPermissionPrinc represents the principal in a permission.
type apiPermissionPrinc struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// apiPermissionOp represents the operation in a permission.
type apiPermissionOp struct {
	Key string `json:"key"`
}

// PermissionDataSourceModel describes the data source data model.
type PermissionDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	SpaceID       types.String `tfsdk:"space_id"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	Operation     types.String `tfsdk:"operation"`
}

// PermissionDataSource implements the atlassian_confluence_space_permission data source.
type PermissionDataSource struct {
	client *atlassian.Client
}

// NewPermissionDataSource returns a new PermissionDataSource instance for provider registration.
func NewPermissionDataSource() datasource.DataSource {
	return &PermissionDataSource{}
}

// Metadata returns the data source type name.
func (d *PermissionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_space_permission"
}

// Schema defines the schema for the space permission data source.
func (d *PermissionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a space permission from Confluence Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The composite identifier (space_id/permission_id).",
				Computed:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the Confluence space.",
				Required:    true,
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal (\"user\" or \"group\").",
				Required:    true,
			},
			"principal_id": schema.StringAttribute{
				Description: "The ID of the principal.",
				Required:    true,
			},
			"operation": schema.StringAttribute{
				Description: "The operation (\"read\", \"write\", or \"admin\").",
				Required:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
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

// Read retrieves space permission data from the Atlassian API.
func (d *PermissionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config PermissionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spaceID := config.SpaceID.ValueString()
	principalType := config.PrincipalType.ValueString()
	principalID := config.PrincipalID.ValueString()
	operation := config.Operation.ValueString()

	var permissions []apiPermission
	err := d.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &permissions)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Space not found",
				fmt.Sprintf("Confluence space %q not found.", spaceID),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read space permissions",
			fmt.Sprintf("Could not read permissions for space %q: %s", spaceID, err.Error()),
		)
		return
	}

	found := false
	for _, perm := range permissions {
		if perm.Principal.Type == principalType &&
			perm.Principal.ID == principalID &&
			perm.Operation.Key == operation {
			config.ID = types.StringValue(fmt.Sprintf("%s/%s", spaceID, perm.ID))
			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.AddError(
			"Permission not found",
			fmt.Sprintf("No %s permission found for %s %q on space %q.", operation, principalType, principalID, spaceID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
