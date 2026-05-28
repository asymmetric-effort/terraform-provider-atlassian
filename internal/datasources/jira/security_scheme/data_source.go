// Package securityscheme implements the atlassian_jira_security_scheme read-only data source.
//
// This data source reads Jira issue security schemes by ID from the Atlassian Cloud REST API.
package securityscheme

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

// apiSecurityLevel represents a single security level in a scheme.
type apiSecurityLevel struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// apiSecurityScheme represents the JSON structure returned by the Atlassian issue security scheme API.
type apiSecurityScheme struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Description    string             `json:"description"`
	Self           string             `json:"self"`
	SecurityLevels []apiSecurityLevel `json:"security_levels,omitempty"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	SecurityLevels types.List   `tfsdk:"security_levels"`
}

// DataSource implements the atlassian_jira_security_scheme data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_security_scheme"
}

// Schema defines the schema for the jira security scheme data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira issue security scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the security scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the security scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the security scheme.",
				Computed:    true,
			},
			"security_levels": schema.ListNestedAttribute{
				Description: "Security levels within the scheme.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the security level.",
							Computed:    true,
						},
						"description": schema.StringAttribute{
							Description: "A description of the security level.",
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

// Read retrieves security scheme data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var ss apiSecurityScheme
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/issuesecurityschemes/%s", identifier), &ss)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Security scheme not found",
				fmt.Sprintf("Jira security scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read security scheme",
			fmt.Sprintf("Could not read Jira security scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(ss.ID)
	config.Name = types.StringValue(ss.Name)
	config.Description = types.StringValue(ss.Description)
	config.SecurityLevels = levelsToState(ctx, ss.SecurityLevels)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// securityLevelObjectType is the attr.Type for security level nested objects.
var securityLevelObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":        types.StringType,
		"description": types.StringType,
	},
}

// levelsToState converts API security levels to the Terraform state list.
func levelsToState(ctx context.Context, levels []apiSecurityLevel) types.List {
	if len(levels) == 0 {
		return types.ListNull(securityLevelObjectType)
	}
	var elems []attr.Value
	for _, l := range levels {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"name":        types.StringType,
				"description": types.StringType,
			},
			map[string]attr.Value{
				"name":        types.StringValue(l.Name),
				"description": types.StringValue(l.Description),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(securityLevelObjectType, elems)
	return list
}
