// Package workflow implements the atlassian_jira_workflow_scheme read-only data source.
//
// This data source reads Jira workflow schemes by ID from the Atlassian Cloud REST API.
package workflow

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

// apiIssueTypeMapping represents a single issue-type-to-workflow mapping in the API.
type apiIssueTypeMapping struct {
	IssueTypeID string `json:"issueType"`
	WorkflowID  string `json:"workflow"`
}

// apiWorkflowScheme represents the JSON structure returned by the Atlassian workflow scheme API.
type apiWorkflowScheme struct {
	ID                string                `json:"id"`
	Name              string                `json:"name"`
	Description       string                `json:"description"`
	DefaultWorkflowID string                `json:"defaultWorkflow,omitempty"`
	IssueTypeMappings []apiIssueTypeMapping `json:"issueTypeMappings,omitempty"`
	Self              string                `json:"self"`
}

// SchemeDataSourceModel describes the workflow scheme data source data model.
type SchemeDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	DefaultWorkflowID types.String `tfsdk:"default_workflow_id"`
	IssueTypeMappings types.List   `tfsdk:"issue_type_mappings"`
}

// SchemeDataSource implements the atlassian_jira_workflow_scheme data source.
type SchemeDataSource struct {
	client *atlassian.Client
}

// NewSchemeDataSource returns a new SchemeDataSource instance for provider registration.
func NewSchemeDataSource() datasource.DataSource {
	return &SchemeDataSource{}
}

// Metadata returns the data source type name.
func (d *SchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow_scheme"
}

// Schema defines the schema for the jira workflow scheme data source.
func (d *SchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira workflow scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the workflow scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the workflow scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the workflow scheme.",
				Computed:    true,
			},
			"default_workflow_id": schema.StringAttribute{
				Description: "The ID of the default workflow for this scheme.",
				Computed:    true,
			},
			"issue_type_mappings": schema.ListNestedAttribute{
				Description: "Mappings of issue types to workflows.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"issue_type_id": schema.StringAttribute{
							Description: "The ID of the issue type.",
							Computed:    true,
						},
						"workflow_id": schema.StringAttribute{
							Description: "The ID of the workflow assigned to this issue type.",
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

// Read retrieves workflow scheme data from the Atlassian API.
func (d *SchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var ws apiWorkflowScheme
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/workflowscheme/%s", identifier), &ws)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Workflow scheme not found",
				fmt.Sprintf("Jira workflow scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read workflow scheme",
			fmt.Sprintf("Could not read Jira workflow scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(ws.ID)
	config.Name = types.StringValue(ws.Name)
	config.Description = types.StringValue(ws.Description)
	config.DefaultWorkflowID = types.StringValue(ws.DefaultWorkflowID)
	config.IssueTypeMappings = dsIssueTypeMappingsToState(ws.IssueTypeMappings)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// dsIssueTypeMappingObjectType is the attr.Type for the issue_type_mappings nested object.
var dsIssueTypeMappingObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"issue_type_id": types.StringType,
		"workflow_id":   types.StringType,
	},
}

// dsIssueTypeMappingsToState converts API issue type mappings to the Terraform state list.
func dsIssueTypeMappingsToState(mappings []apiIssueTypeMapping) types.List {
	if len(mappings) == 0 {
		return types.ListNull(dsIssueTypeMappingObjectType)
	}
	var elems []attr.Value
	for _, m := range mappings {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"issue_type_id": types.StringType,
				"workflow_id":   types.StringType,
			},
			map[string]attr.Value{
				"issue_type_id": types.StringValue(m.IssueTypeID),
				"workflow_id":   types.StringValue(m.WorkflowID),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(dsIssueTypeMappingObjectType, elems)
	return list
}
