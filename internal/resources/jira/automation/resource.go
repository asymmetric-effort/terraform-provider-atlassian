// Package automation implements the atlassian_jira_automation_rule managed resource.
//
// This resource manages Jira automation rules through the Atlassian
// Cloud REST API. It supports full CRUD operations and state import
// via rule ID.
package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

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

// apiRuleCreate represents the JSON body for creating a rule.
type apiRuleCreate struct {
	Name          string          `json:"name"`
	State         string          `json:"state,omitempty"`
	TriggerType   string          `json:"triggerType"`
	TriggerConfig json.RawMessage `json:"triggerConfig,omitempty"`
	Conditions    json.RawMessage `json:"conditions,omitempty"`
	Actions       json.RawMessage `json:"actions"`
}

// apiRuleUpdate represents the JSON body for updating a rule.
type apiRuleUpdate struct {
	Name          string          `json:"name,omitempty"`
	State         string          `json:"state,omitempty"`
	TriggerType   string          `json:"triggerType,omitempty"`
	TriggerConfig json.RawMessage `json:"triggerConfig,omitempty"`
	Conditions    json.RawMessage `json:"conditions,omitempty"`
	Actions       json.RawMessage `json:"actions,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	State         types.String `tfsdk:"state"`
	TriggerType   types.String `tfsdk:"trigger_type"`
	TriggerConfig types.String `tfsdk:"trigger_config"`
	Conditions    types.String `tfsdk:"conditions"`
	Actions       types.String `tfsdk:"actions"`
}

// Resource implements the atlassian_jira_automation_rule managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_automation_rule"
}

// Schema defines the schema for the jira automation rule resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira automation rule in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the automation rule, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the automation rule.",
				Required:    true,
			},
			"state": schema.StringAttribute{
				Description: "The state of the automation rule. Must be \"enabled\" or \"disabled\".",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"trigger_type": schema.StringAttribute{
				Description: "The type of trigger for the automation rule (e.g., \"issue_created\", \"field_value_changed\").",
				Required:    true,
			},
			"trigger_config": schema.StringAttribute{
				Description: "JSON string containing the trigger configuration.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"conditions": schema.StringAttribute{
				Description: "JSON string containing the rule conditions.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"actions": schema.StringAttribute{
				Description: "JSON string containing the rule actions.",
				Required:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.client = client
}

// Create provisions a new Jira automation rule.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiRuleCreate{
		Name:        plan.Name.ValueString(),
		TriggerType: plan.TriggerType.ValueString(),
		Actions:     json.RawMessage(plan.Actions.ValueString()),
	}
	if !plan.State.IsNull() && !plan.State.IsUnknown() {
		body.State = plan.State.ValueString()
	}
	if !plan.TriggerConfig.IsNull() && !plan.TriggerConfig.IsUnknown() {
		body.TriggerConfig = json.RawMessage(plan.TriggerConfig.ValueString())
	}
	if !plan.Conditions.IsNull() && !plan.Conditions.IsUnknown() {
		body.Conditions = json.RawMessage(plan.Conditions.ValueString())
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiRule
	err := r.client.Post(ctx, "/rest/api/3/automation/rule", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid automation rule configuration",
					fmt.Sprintf("The automation rule configuration is invalid. Check that the trigger type %q is valid and the action configuration is correct: %s",
						plan.TriggerType.ValueString(), err.Error()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create automation rules. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create automation rule",
			fmt.Sprintf("Could not create automation rule %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	r.mapAPIToState(&plan, &created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var rule apiRule
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", state.ID.ValueString()), &rule)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read automation rule",
			fmt.Sprintf("Could not read automation rule %q: %s. Verify the rule exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	r.mapAPIToState(&state, &rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira automation rule.
func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiRuleUpdate{
		Name:        plan.Name.ValueString(),
		TriggerType: plan.TriggerType.ValueString(),
		Actions:     json.RawMessage(plan.Actions.ValueString()),
	}
	if !plan.State.IsNull() && !plan.State.IsUnknown() {
		body.State = plan.State.ValueString()
	}
	if !plan.TriggerConfig.IsNull() && !plan.TriggerConfig.IsUnknown() {
		body.TriggerConfig = json.RawMessage(plan.TriggerConfig.ValueString())
	}
	if !plan.Conditions.IsNull() && !plan.Conditions.IsUnknown() {
		body.Conditions = json.RawMessage(plan.Conditions.ValueString())
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiRule
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Automation rule not found",
					fmt.Sprintf("Automation rule with ID %q not found. The rule may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid automation rule configuration",
					fmt.Sprintf("The automation rule configuration is invalid. Check that the trigger type and action configuration are correct: %s", err.Error()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update automation rules. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update automation rule",
			fmt.Sprintf("Could not update automation rule with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	r.mapAPIToState(&plan, &updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira automation rule.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete automation rule %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete automation rule",
			fmt.Sprintf("Could not delete automation rule with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing automation rule by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// mapAPIToState maps API response fields to the Terraform state model.
func (r *Resource) mapAPIToState(model *ResourceModel, api *apiRule) {
	model.ID = types.StringValue(api.ID)
	model.Name = types.StringValue(api.Name)
	model.State = types.StringValue(api.State)
	model.TriggerType = types.StringValue(api.TriggerType)
	if len(api.TriggerConfig) > 0 && string(api.TriggerConfig) != "null" {
		model.TriggerConfig = types.StringValue(string(api.TriggerConfig))
	} else {
		model.TriggerConfig = types.StringValue("")
	}
	if len(api.Conditions) > 0 && string(api.Conditions) != "null" {
		model.Conditions = types.StringValue(string(api.Conditions))
	} else {
		model.Conditions = types.StringValue("")
	}
	model.Actions = types.StringValue(string(api.Actions))
}
