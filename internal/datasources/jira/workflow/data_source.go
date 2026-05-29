// Package workflow implements the atlassian_jira_workflow read-only data source.
//
// This data source reads Jira workflows by ID from the Atlassian Cloud REST API.
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

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiWorkflowStatus represents a single status in a workflow.
type apiWorkflowStatus struct {
	Name     string `json:"name"`
	Category string `json:"category,omitempty"`
}

// apiWorkflowTransition represents a single transition in a workflow.
type apiWorkflowTransition struct {
	Name       string `json:"name"`
	FromStatus string `json:"fromStatus,omitempty"`
	ToStatus   string `json:"toStatus"`
}

// apiWorkflow represents the JSON structure returned by the Atlassian workflow API.
type apiWorkflow struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Self        string                  `json:"self"`
	Statuses    []apiWorkflowStatus     `json:"statuses,omitempty"`
	Transitions []apiWorkflowTransition `json:"transitions,omitempty"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Statuses    types.List   `tfsdk:"statuses"`
	Transitions types.List   `tfsdk:"transitions"`
}

// DataSource implements the atlassian_jira_workflow data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_workflow"
}

// Schema defines the schema for the jira workflow data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira workflow from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the workflow.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the workflow.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the workflow.",
				Computed:    true,
			},
			"statuses": schema.ListNestedAttribute{
				Description: "The statuses defined in the workflow.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the status.",
							Computed:    true,
						},
						"category": schema.StringAttribute{
							Description: "The status category (e.g., \"new\", \"indeterminate\", \"done\").",
							Computed:    true,
						},
					},
				},
			},
			"transitions": schema.ListNestedAttribute{
				Description: "The transitions defined in the workflow.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the transition.",
							Computed:    true,
						},
						"from_status": schema.StringAttribute{
							Description: "The source status name. Empty for the initial transition.",
							Computed:    true,
						},
						"to_status": schema.StringAttribute{
							Description: "The target status name.",
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

// Read retrieves workflow data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var wf apiWorkflow
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", identifier), &wf)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Workflow not found",
				fmt.Sprintf("Jira workflow %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read workflow",
			fmt.Sprintf("Could not read Jira workflow %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(wf.ID)
	config.Name = types.StringValue(wf.Name)
	config.Description = types.StringValue(wf.Description)
	config.Statuses = statusesToState(wf.Statuses)
	config.Transitions = transitionsToState(wf.Transitions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// statusObjectType is the attr.Type for the status nested object.
var statusObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":     types.StringType,
		"category": types.StringType,
	},
}

// transitionObjectType is the attr.Type for the transition nested object.
var transitionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":        types.StringType,
		"from_status": types.StringType,
		"to_status":   types.StringType,
	},
}

// statusesToState converts API statuses to the Terraform state list.
func statusesToState(statuses []apiWorkflowStatus) types.List {
	if len(statuses) == 0 {
		return types.ListNull(statusObjectType)
	}
	var elems []attr.Value
	for _, s := range statuses {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"name":     types.StringType,
				"category": types.StringType,
			},
			map[string]attr.Value{
				"name":     types.StringValue(s.Name),
				"category": types.StringValue(s.Category),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(statusObjectType, elems)
	return list
}

// transitionsToState converts API transitions to the Terraform state list.
func transitionsToState(transitions []apiWorkflowTransition) types.List {
	if len(transitions) == 0 {
		return types.ListNull(transitionObjectType)
	}
	var elems []attr.Value
	for _, t := range transitions {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"name":        types.StringType,
				"from_status": types.StringType,
				"to_status":   types.StringType,
			},
			map[string]attr.Value{
				"name":        types.StringValue(t.Name),
				"from_status": types.StringValue(t.FromStatus),
				"to_status":   types.StringValue(t.ToStatus),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(transitionObjectType, elems)
	return list
}
