// Package dashboard implements the atlassian_jira_dashboard read-only data source.
//
// This data source reads Jira dashboards by ID from the Atlassian Cloud REST API.
package dashboard

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiSharePermission represents a share permission entry in the Atlassian API.
type apiSharePermission struct {
	Type      string `json:"type"`
	Parameter string `json:"parameter,omitempty"`
}

// apiDashboard represents the JSON structure returned by the Atlassian dashboard API.
type apiDashboard struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	Self             string               `json:"self"`
	SharePermissions []apiSharePermission `json:"sharePermissions,omitempty"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	SharePermissions types.List   `tfsdk:"share_permissions"`
}

// sharePermissionObjectType is the attr.Type for the share permission nested object.
var sharePermissionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"type":      types.StringType,
		"parameter": types.StringType,
	},
}

// sharePermissionsToState converts API share permissions to the Terraform state list.
func sharePermissionsToState(ctx context.Context, perms []apiSharePermission) types.List {
	if len(perms) == 0 {
		return types.ListNull(sharePermissionObjectType)
	}
	var elems []attr.Value
	for _, p := range perms {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"type":      types.StringType,
				"parameter": types.StringType,
			},
			map[string]attr.Value{
				"type":      types.StringValue(p.Type),
				"parameter": types.StringValue(p.Parameter),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(sharePermissionObjectType, elems)
	return list
}

// DataSource implements the atlassian_jira_dashboard data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_dashboard"
}

// Schema defines the schema for the jira dashboard data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira dashboard from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the dashboard.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the dashboard.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the dashboard.",
				Computed:    true,
			},
			"share_permissions": schema.ListNestedAttribute{
				Description: "Share permissions controlling who can view the dashboard.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Description: "The type of share permission.",
							Computed:    true,
						},
						"parameter": schema.StringAttribute{
							Description: "The parameter for the share permission.",
							Computed:    true,
						},
					},
				},
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

// Read retrieves dashboard data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var db apiDashboard
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/dashboard/%s", identifier), &db)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Dashboard not found",
				fmt.Sprintf("Jira dashboard %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read dashboard",
			fmt.Sprintf("Could not read Jira dashboard %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(db.ID)
	config.Name = types.StringValue(db.Name)
	config.Description = types.StringValue(db.Description)
	config.SharePermissions = sharePermissionsToState(ctx, db.SharePermissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
