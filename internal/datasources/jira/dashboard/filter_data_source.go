// Package dashboard implements the atlassian_jira_filter read-only data source.
//
// This data source reads Jira filters by ID from the Atlassian Cloud REST API.
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

// Ensure the FilterDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &FilterDataSource{}

// apiFilterSharePermission represents a share permission entry in the filter API.
type apiFilterSharePermission struct {
	Type      string `json:"type"`
	Parameter string `json:"parameter,omitempty"`
}

// apiFilter represents the JSON structure returned by the Atlassian filter API.
type apiFilter struct {
	ID               string                     `json:"id"`
	Name             string                     `json:"name"`
	Description      string                     `json:"description"`
	JQL              string                     `json:"jql"`
	Self             string                     `json:"self"`
	SharePermissions []apiFilterSharePermission `json:"sharePermissions,omitempty"`
}

// FilterDataSourceModel describes the filter data source data model.
type FilterDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	JQL              types.String `tfsdk:"jql"`
	SharePermissions types.List   `tfsdk:"share_permissions"`
}

// filterSharePermissionObjectType is the attr.Type for filter share permission nested objects.
var filterSharePermissionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"type":      types.StringType,
		"parameter": types.StringType,
	},
}

// filterSharePermissionsToState converts API filter share permissions to the Terraform state list.
func filterSharePermissionsToState(ctx context.Context, perms []apiFilterSharePermission) types.List {
	if len(perms) == 0 {
		return types.ListNull(filterSharePermissionObjectType)
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
	list, _ := types.ListValue(filterSharePermissionObjectType, elems)
	return list
}

// FilterDataSource implements the atlassian_jira_filter data source.
type FilterDataSource struct {
	client *atlassian.Client
}

// NewFilterDataSource returns a new FilterDataSource instance for provider registration.
func NewFilterDataSource() datasource.DataSource {
	return &FilterDataSource{}
}

// Metadata returns the data source type name.
func (d *FilterDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_filter"
}

// Schema defines the schema for the jira filter data source.
func (d *FilterDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira filter from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the filter.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the filter.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the filter.",
				Computed:    true,
			},
			"jql": schema.StringAttribute{
				Description: "The JQL query for the filter.",
				Computed:    true,
			},
			"share_permissions": schema.ListNestedAttribute{
				Description: "Share permissions controlling who can view the filter.",
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
func (d *FilterDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves filter data from the Atlassian API.
func (d *FilterDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config FilterDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var f apiFilter
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/filter/%s", identifier), &f)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Filter not found",
				fmt.Sprintf("Jira filter %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read filter",
			fmt.Sprintf("Could not read Jira filter %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(f.ID)
	config.Name = types.StringValue(f.Name)
	config.Description = types.StringValue(f.Description)
	config.JQL = types.StringValue(f.JQL)
	config.SharePermissions = filterSharePermissionsToState(ctx, f.SharePermissions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
