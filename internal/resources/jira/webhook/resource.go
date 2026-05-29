// Package webhook implements the atlassian_jira_webhook managed resource.
//
// This resource manages Jira webhooks through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via webhook ID.
package webhook

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiWebhook represents the JSON structure returned by the Atlassian webhook API.
type apiWebhook struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	JQLFilter string   `json:"jqlFilter,omitempty"`
	Enabled   bool     `json:"enabled"`
	Self      string   `json:"self"`
}

// apiWebhookCreate represents the JSON body for creating a webhook.
type apiWebhookCreate struct {
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	JQLFilter string   `json:"jqlFilter,omitempty"`
	Enabled   bool     `json:"enabled"`
}

// ResourceModel describes the webhook resource data model.
type ResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	URL       types.String `tfsdk:"url"`
	Events    types.List   `tfsdk:"events"`
	JQLFilter types.String `tfsdk:"jql_filter"`
	Enabled   types.Bool   `tfsdk:"enabled"`
}

// Resource implements the atlassian_jira_webhook managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_webhook"
}

// Schema defines the schema for the jira webhook resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira webhook in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the webhook, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the webhook.",
				Required:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL that the webhook posts event data to.",
				Required:    true,
			},
			"events": schema.ListAttribute{
				Description: "The list of Jira event types the webhook listens for (e.g., jira:issue_created, jira:issue_updated).",
				Required:    true,
				ElementType: types.StringType,
			},
			"jql_filter": schema.StringAttribute{
				Description: "An optional JQL filter to restrict which issues trigger the webhook.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the webhook is enabled. Defaults to true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
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

// Create provisions a new Jira webhook.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var events []string
	plan.Events.ElementsAs(ctx, &events, false)

	body := apiWebhookCreate{
		Name:    plan.Name.ValueString(),
		URL:     plan.URL.ValueString(),
		Events:  events,
		Enabled: plan.Enabled.ValueBool(),
	}
	if !plan.JQLFilter.IsNull() && !plan.JQLFilter.IsUnknown() {
		body.JQLFilter = plan.JQLFilter.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiWebhook
	err := r.client.Post(ctx, "/rest/api/3/webhook", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid webhook",
					fmt.Sprintf("The webhook %q is invalid. Verify the URL, events, and JQL filter are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create webhooks. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create webhook",
			fmt.Sprintf("Could not create webhook with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.URL = types.StringValue(created.URL)

	eventsList, _ := types.ListValueFrom(ctx, types.StringType, created.Events)
	plan.Events = eventsList
	plan.JQLFilter = types.StringValue(created.JQLFilter)
	plan.Enabled = types.BoolValue(created.Enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var wh apiWebhook
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/webhook/%s", state.ID.ValueString()), &wh)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read webhook",
			fmt.Sprintf("Could not read webhook %q: %s. Verify the webhook exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(wh.ID)
	state.Name = types.StringValue(wh.Name)
	state.URL = types.StringValue(wh.URL)

	eventsList, _ := types.ListValueFrom(ctx, types.StringType, wh.Events)
	state.Events = eventsList
	state.JQLFilter = types.StringValue(wh.JQLFilter)
	state.Enabled = types.BoolValue(wh.Enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira webhook.
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

	var events []string
	plan.Events.ElementsAs(ctx, &events, false)

	body := apiWebhookCreate{
		Name:    plan.Name.ValueString(),
		URL:     plan.URL.ValueString(),
		Events:  events,
		Enabled: plan.Enabled.ValueBool(),
	}
	if !plan.JQLFilter.IsNull() && !plan.JQLFilter.IsUnknown() {
		body.JQLFilter = plan.JQLFilter.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiWebhook
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/webhook/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Webhook not found",
					fmt.Sprintf("Webhook with ID %q not found. The webhook may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid webhook",
					fmt.Sprintf("The webhook update for ID %q is invalid. Verify the URL, events, and JQL filter are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update webhooks. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update webhook",
			fmt.Sprintf("Could not update webhook with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.URL = types.StringValue(updated.URL)

	eventsList, _ := types.ListValueFrom(ctx, types.StringType, updated.Events)
	plan.Events = eventsList
	plan.JQLFilter = types.StringValue(updated.JQLFilter)
	plan.Enabled = types.BoolValue(updated.Enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira webhook.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/webhook/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete webhook %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete webhook",
			fmt.Sprintf("Could not delete webhook with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing webhook by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
