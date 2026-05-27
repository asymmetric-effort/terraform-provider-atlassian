// Package role implements the atlassian_role data source.
//
// This data source reads an existing Atlassian Cloud role by ID or name.
package role

import (
	"context"
	"encoding/json"
	"fmt"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure DataSource satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// DataSource implements the atlassian_role data source.
type DataSource struct {
	client *atlassian.Client
}

// DataSourceModel describes the data source data model for an Atlassian role.
type DataSourceModel struct {
	RoleID      types.String `tfsdk:"role_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Scope       types.String `tfsdk:"scope"`
}

// apiRoleResponse represents the JSON response from the Atlassian role API.
type apiRoleResponse struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope,omitempty"`
}

// NewDataSource returns a new instance of the role data source.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

// Schema defines the schema for the role data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Atlassian Cloud role by ID or name.",
		Attributes: map[string]schema.Attribute{
			"role_id": schema.StringAttribute{
				Description: "The unique identifier of the role. Provide either role_id or name.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the role. Provide either role_id or name.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the role.",
				Computed:    true,
			},
			"scope": schema.StringAttribute{
				Description: "The scope of the role (\"org\" or \"product\").",
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

// Read retrieves an Atlassian role by ID or name.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roleID := config.RoleID.ValueString()
	roleName := config.Name.ValueString()

	if roleID == "" && roleName == "" {
		resp.Diagnostics.AddError(
			"Missing required argument",
			"Either 'role_id' or 'name' must be specified to look up a role.",
		)
		return
	}

	var apiResp apiRoleResponse

	if roleID != "" {
		err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/role/%s", roleID), &apiResp)
		if err != nil {
			if apiErr, ok := err.(*atlassian.APIError); ok {
				switch apiErr.StatusCode {
				case 404:
					resp.Diagnostics.AddError(
						"Role not found",
						fmt.Sprintf("No role found with ID %s. Verify the role exists and the identifier is correct.", roleID),
					)
					return
				case 403:
					resp.Diagnostics.AddError(
						"Permission denied",
						"The authenticated user does not have permission to read this role. "+
							"Ensure the service account has the required permissions.",
					)
					return
				}
			}
			resp.Diagnostics.AddError(
				"Failed to read role",
				"Could not read role: "+err.Error(),
			)
			return
		}
	} else {
		found, err := d.findRoleByName(ctx, roleName)
		if err != nil {
			if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 404 {
				resp.Diagnostics.AddError(
					"Role not found",
					fmt.Sprintf("No role found with name %q. Verify the role exists and the name is correct.", roleName),
				)
				return
			}
			resp.Diagnostics.AddError(
				"Failed to read role",
				"Could not read role: "+err.Error(),
			)
			return
		}
		apiResp = found
	}

	config.RoleID = types.StringValue(fmt.Sprintf("%d", apiResp.ID))
	config.Name = types.StringValue(apiResp.Name)
	config.Description = types.StringValue(apiResp.Description)
	config.Scope = types.StringValue(apiResp.Scope)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// findRoleByName lists all roles and returns the one matching the given name.
func (d *DataSource) findRoleByName(ctx context.Context, name string) (apiRoleResponse, error) {
	var roles []json.RawMessage
	err := d.client.Get(ctx, "/rest/api/3/role", &roles)
	if err != nil {
		return apiRoleResponse{}, err
	}

	for _, raw := range roles {
		var role apiRoleResponse
		if err := json.Unmarshal(raw, &role); err != nil {
			continue
		}
		if role.Name == name {
			return role, nil
		}
	}

	return apiRoleResponse{}, &atlassian.APIError{
		StatusCode: 404,
		Message:    fmt.Sprintf("no role found with name %q", name),
		Resource:   "role",
		Action:     "read",
	}
}
