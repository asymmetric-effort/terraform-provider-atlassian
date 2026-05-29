// Package screen implements the atlassian_jira_issue_type_screen_scheme read-only data source.
//
// This data source reads Jira issue type screen schemes by ID from the Atlassian Cloud REST API.
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

// Ensure the IssueTypeScreenSchemeDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &IssueTypeScreenSchemeDataSource{}

// apiIssueTypeScreenMapping represents a single issue-type-to-screen-scheme mapping in the API.
type apiIssueTypeScreenMapping struct {
	IssueTypeID    string `json:"issueTypeId"`
	ScreenSchemeID string `json:"screenSchemeId"`
}

// apiIssueTypeScreenScheme represents the JSON structure returned by the Atlassian issue type screen scheme API.
type apiIssueTypeScreenScheme struct {
	ID                string                      `json:"id"`
	Name              string                      `json:"name"`
	Description       string                      `json:"description"`
	IssueTypeMappings []apiIssueTypeScreenMapping `json:"issueTypeMappings,omitempty"`
}

// IssueTypeScreenSchemeDataSourceModel describes the data source data model for issue type screen schemes.
type IssueTypeScreenSchemeDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	IssueTypeMappings types.List   `tfsdk:"issue_type_mappings"`
}

// IssueTypeScreenSchemeDataSource implements the atlassian_jira_issue_type_screen_scheme data source.
type IssueTypeScreenSchemeDataSource struct {
	client *atlassian.Client
}

// NewIssueTypeScreenSchemeDataSource returns a new IssueTypeScreenSchemeDataSource instance for provider registration.
func NewIssueTypeScreenSchemeDataSource() datasource.DataSource {
	return &IssueTypeScreenSchemeDataSource{}
}

// Metadata returns the data source type name.
func (d *IssueTypeScreenSchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_issue_type_screen_scheme"
}

// Schema defines the schema for the jira issue type screen scheme data source.
func (d *IssueTypeScreenSchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira issue type screen scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the issue type screen scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the issue type screen scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the issue type screen scheme.",
				Computed:    true,
			},
			"issue_type_mappings": schema.ListNestedAttribute{
				Description: "Mappings of issue types to screen schemes.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"issue_type_id": schema.StringAttribute{
							Description: "The ID of the issue type.",
							Computed:    true,
						},
						"screen_scheme_id": schema.StringAttribute{
							Description: "The ID of the screen scheme assigned to this issue type.",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *IssueTypeScreenSchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves issue type screen scheme data from the Atlassian API.
func (d *IssueTypeScreenSchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config IssueTypeScreenSchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var scheme apiIssueTypeScreenScheme
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/issuetypescreenscheme/%s", identifier), &scheme)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Issue type screen scheme not found",
				fmt.Sprintf("Jira issue type screen scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read issue type screen scheme",
			fmt.Sprintf("Could not read Jira issue type screen scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(scheme.ID)
	config.Name = types.StringValue(scheme.Name)
	config.Description = types.StringValue(scheme.Description)
	config.IssueTypeMappings = dsIssueTypeScreenMappingsToState(scheme.IssueTypeMappings)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// dsIssueTypeScreenMappingObjectType is the attr.Type for the issue_type_mappings nested object.
var dsIssueTypeScreenMappingObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"issue_type_id":    types.StringType,
		"screen_scheme_id": types.StringType,
	},
}

// dsIssueTypeScreenMappingsToState converts API issue type screen mappings to the Terraform state list.
func dsIssueTypeScreenMappingsToState(mappings []apiIssueTypeScreenMapping) types.List {
	if len(mappings) == 0 {
		return types.ListNull(dsIssueTypeScreenMappingObjectType)
	}
	var elems []attr.Value
	for _, m := range mappings {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"issue_type_id":    types.StringType,
				"screen_scheme_id": types.StringType,
			},
			map[string]attr.Value{
				"issue_type_id":    types.StringValue(m.IssueTypeID),
				"screen_scheme_id": types.StringValue(m.ScreenSchemeID),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(dsIssueTypeScreenMappingObjectType, elems)
	return list
}
