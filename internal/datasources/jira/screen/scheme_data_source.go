// Package screen implements the atlassian_jira_screen_scheme read-only data source.
//
// This data source reads Jira screen schemes by ID from the Atlassian Cloud REST API.
package screen

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

// Ensure the SchemeDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &SchemeDataSource{}

// apiScreenMapping represents a single operation-to-screen mapping in the API.
type apiScreenMapping struct {
	Operation string `json:"operation"`
	ScreenID  string `json:"screenId"`
}

// apiScreenScheme represents the JSON structure returned by the Atlassian screen scheme API.
type apiScreenScheme struct {
	ID          int                `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Screens     []apiScreenMapping `json:"screens,omitempty"`
}

// SchemeDataSourceModel describes the data source data model for screen schemes.
type SchemeDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Screens     types.List   `tfsdk:"screens"`
}

// SchemeDataSource implements the atlassian_jira_screen_scheme data source.
type SchemeDataSource struct {
	client *atlassian.Client
}

// NewSchemeDataSource returns a new SchemeDataSource instance for provider registration.
func NewSchemeDataSource() datasource.DataSource {
	return &SchemeDataSource{}
}

// Metadata returns the data source type name.
func (d *SchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_scheme"
}

// Schema defines the schema for the jira screen scheme data source.
func (d *SchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira screen scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the screen scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the screen scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the screen scheme.",
				Computed:    true,
			},
			"screens": schema.ListNestedAttribute{
				Description: "Mappings of operations to screen IDs.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"operation": schema.StringAttribute{
							Description: "The operation mapped to the screen (default, create, edit, or view).",
							Computed:    true,
						},
						"screen_id": schema.StringAttribute{
							Description: "The ID of the screen assigned to this operation.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *SchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves screen scheme data from the Atlassian API.
func (d *SchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var scheme apiScreenScheme
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/screenscheme/%s", identifier), &scheme)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Screen scheme not found",
				fmt.Sprintf("Jira screen scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read screen scheme",
			fmt.Sprintf("Could not read Jira screen scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(fmt.Sprintf("%d", scheme.ID))
	config.Name = types.StringValue(scheme.Name)
	config.Description = types.StringValue(scheme.Description)
	config.Screens = dsScreenMappingsToState(scheme.Screens)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// dsScreenMappingObjectType is the attr.Type for the screens nested object.
var dsScreenMappingObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"operation": types.StringType,
		"screen_id": types.StringType,
	},
}

// dsScreenMappingsToState converts API screen mappings to the Terraform state list.
func dsScreenMappingsToState(mappings []apiScreenMapping) types.List {
	if len(mappings) == 0 {
		return types.ListNull(dsScreenMappingObjectType)
	}
	var elems []attr.Value
	for _, m := range mappings {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"operation": types.StringType,
				"screen_id": types.StringType,
			},
			map[string]attr.Value{
				"operation": types.StringValue(m.Operation),
				"screen_id": types.StringValue(m.ScreenID),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(dsScreenMappingObjectType, elems)
	return list
}
