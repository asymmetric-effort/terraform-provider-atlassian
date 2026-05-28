// Package automation implements the atlassian_jira_automation_rule read-only data source.
//
// This data source reads Jira automation rules by ID from the
// Atlassian Cloud REST API.
package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiRule represents the JSON structure returned by the Atlassian automation rule API.
type apiRule struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	State         string          `json:"state"`
	TriggerType   string          `json:"triggerType"`
	TriggerConfig json.RawMessage `json:"triggerConfig,omitempty"`
	Conditions    json.RawMessage `json:"conditions,omitempty"`
	Actions       json.RawMessage `json:"actions"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	State         types.String `tfsdk:"state"`
	TriggerType   types.String `tfsdk:"trigger_type"`
	TriggerConfig types.String `tfsdk:"trigger_config"`
	Conditions    types.String `tfsdk:"conditions"`
	Actions       types.String `tfsdk:"actions"`
}

// DataSource implements the atlassian_jira_automation_rule data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_automation_rule"
}

// Schema defines the schema for the jira automation rule data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira automation rule from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the automation rule.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the automation rule.",
				Computed:    true,
			},
			"state": schema.StringAttribute{
				Description: "The state of the automation rule (\"enabled\" or \"disabled\").",
				Computed:    true,
			},
			"trigger_type": schema.StringAttribute{
				Description: "The type of trigger for the automation rule.",
				Computed:    true,
			},
			"trigger_config": schema.StringAttribute{
				Description: "JSON string containing the trigger configuration.",
				Computed:    true,
			},
			"conditions": schema.StringAttribute{
				Description: "JSON string containing the rule conditions.",
				Computed:    true,
			},
			"actions": schema.StringAttribute{
				Description: "JSON string containing the rule actions.",
				Computed:    true,
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

// Read retrieves automation rule data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.ID.IsNull() || config.ID.IsUnknown() {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"The id attribute must be specified to look up a Jira automation rule.",
		)
		return
	}

	identifier := config.ID.ValueString()

	var rule apiRule
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", identifier), &rule)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Automation rule not found",
				fmt.Sprintf("Jira automation rule %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read automation rule",
			fmt.Sprintf("Could not read Jira automation rule %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(rule.ID)
	config.Name = types.StringValue(rule.Name)
	config.State = types.StringValue(rule.State)
	config.TriggerType = types.StringValue(rule.TriggerType)
	if len(rule.TriggerConfig) > 0 && string(rule.TriggerConfig) != "null" {
		config.TriggerConfig = types.StringValue(string(rule.TriggerConfig))
	} else {
		config.TriggerConfig = types.StringValue("")
	}
	if len(rule.Conditions) > 0 && string(rule.Conditions) != "null" {
		config.Conditions = types.StringValue(string(rule.Conditions))
	} else {
		config.Conditions = types.StringValue("")
	}
	config.Actions = types.StringValue(string(rule.Actions))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
