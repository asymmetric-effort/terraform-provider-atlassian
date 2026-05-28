// Package notificationscheme implements the atlassian_jira_notification_scheme managed resource.
//
// This resource manages Jira notification schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via notification scheme ID.
package notificationscheme

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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

// apiNotificationEvent represents a single notification event in a scheme.
type apiNotificationEvent struct {
	EventType     string `json:"event_type"`
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
}

// apiNotificationScheme represents the JSON structure returned by the Atlassian notification scheme API.
type apiNotificationScheme struct {
	ID                 string                 `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	Self               string                 `json:"self"`
	NotificationEvents []apiNotificationEvent `json:"notification_events,omitempty"`
}

// apiNotificationSchemeCreate represents the JSON body for creating a notification scheme.
type apiNotificationSchemeCreate struct {
	Name               string                 `json:"name"`
	Description        string                 `json:"description,omitempty"`
	NotificationEvents []apiNotificationEvent `json:"notification_events,omitempty"`
}

// apiNotificationSchemeUpdate represents the JSON body for updating a notification scheme.
type apiNotificationSchemeUpdate struct {
	Name               string                 `json:"name,omitempty"`
	Description        string                 `json:"description,omitempty"`
	NotificationEvents []apiNotificationEvent `json:"notification_events,omitempty"`
}

// NotificationEvent describes a single notification event in the Terraform model.
type NotificationEvent struct {
	EventType     string `tfsdk:"event_type"`
	RecipientType string `tfsdk:"recipient_type"`
	RecipientID   string `tfsdk:"recipient_id"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	NotificationEvents types.List   `tfsdk:"notification_events"`
}

// Resource implements the atlassian_jira_notification_scheme managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_notification_scheme"
}

// Schema defines the schema for the jira notification scheme resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira notification scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the notification scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the notification scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the notification scheme.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"notification_events": schema.ListNestedAttribute{
				Description: "Notification events within the scheme.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"event_type": schema.StringAttribute{
							Description: "The type of event that triggers the notification.",
							Required:    true,
						},
						"recipient_type": schema.StringAttribute{
							Description: "The type of recipient for the notification.",
							Required:    true,
						},
						"recipient_id": schema.StringAttribute{
							Description: "The identifier of the notification recipient.",
							Required:    true,
						},
					},
				},
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

// Create provisions a new Jira notification scheme.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiNotificationSchemeCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.NotificationEvents = eventsFromPlan(ctx, plan.NotificationEvents)
	bodyBytes, _ := json.Marshal(body)

	var created apiNotificationScheme
	err := r.client.Post(ctx, "/rest/api/3/notificationscheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate notification scheme name",
					fmt.Sprintf("A notification scheme with name %q already exists. Each notification scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid notification scheme configuration",
					fmt.Sprintf("The notification scheme configuration for %q is invalid. Verify the scheme name and notification events are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create notification schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create notification scheme",
			fmt.Sprintf("Could not create notification scheme with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.NotificationEvents = eventsToState(ctx, created.NotificationEvents)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var ns apiNotificationScheme
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/notificationscheme/%s", state.ID.ValueString()), &ns)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read notification scheme",
			fmt.Sprintf("Could not read notification scheme %q: %s. Verify the notification scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(ns.ID)
	state.Name = types.StringValue(ns.Name)
	state.Description = types.StringValue(ns.Description)
	state.NotificationEvents = eventsToState(ctx, ns.NotificationEvents)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira notification scheme.
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

	body := apiNotificationSchemeUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	body.NotificationEvents = eventsFromPlan(ctx, plan.NotificationEvents)
	bodyBytes, _ := json.Marshal(body)

	var updated apiNotificationScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/notificationscheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Notification scheme not found",
					fmt.Sprintf("Notification scheme with ID %q not found. The notification scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid notification scheme configuration",
					fmt.Sprintf("The notification scheme update for ID %q is invalid. Verify the scheme name and notification events are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update notification schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update notification scheme",
			fmt.Sprintf("Could not update notification scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.NotificationEvents = eventsToState(ctx, updated.NotificationEvents)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira notification scheme.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/notificationscheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete notification scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete notification scheme",
			fmt.Sprintf("Could not delete notification scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing notification scheme by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// notificationEventObjectType is the attr.Type for the notification event nested object.
var notificationEventObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"event_type":     types.StringType,
		"recipient_type": types.StringType,
		"recipient_id":   types.StringType,
	},
}

// eventsFromPlan converts the Terraform notification_events list to API format.
func eventsFromPlan(ctx context.Context, events types.List) []apiNotificationEvent {
	if events.IsNull() || events.IsUnknown() {
		return nil
	}
	var eventModels []NotificationEvent
	events.ElementsAs(ctx, &eventModels, false)
	var result []apiNotificationEvent
	for _, e := range eventModels {
		result = append(result, apiNotificationEvent{
			EventType:     e.EventType,
			RecipientType: e.RecipientType,
			RecipientID:   e.RecipientID,
		})
	}
	return result
}

// eventsToState converts API notification events to the Terraform state list.
func eventsToState(ctx context.Context, events []apiNotificationEvent) types.List {
	if len(events) == 0 {
		return types.ListNull(notificationEventObjectType)
	}
	var elems []attr.Value
	for _, e := range events {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"event_type":     types.StringType,
				"recipient_type": types.StringType,
				"recipient_id":   types.StringType,
			},
			map[string]attr.Value{
				"event_type":     types.StringValue(e.EventType),
				"recipient_type": types.StringValue(e.RecipientType),
				"recipient_id":   types.StringValue(e.RecipientID),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(notificationEventObjectType, elems)
	return list
}
