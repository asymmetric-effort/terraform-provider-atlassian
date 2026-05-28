// Package subscriber implements the atlassian_statuspage_subscriber managed resource.
//
// This resource manages Statuspage subscribers through the Atlassian
// Statuspage REST API (v1). It supports full CRUD operations and
// state import via subscriber ID.
package subscriber

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

// apiSubscriber represents the JSON structure returned by the subscriber API.
type apiSubscriber struct {
	ID           string   `json:"id"`
	PageID       string   `json:"page_id"`
	Email        string   `json:"email"`
	Endpoint     string   `json:"endpoint"`
	ComponentIDs []string `json:"component_ids"`
}

// apiSubscriberCreate represents the JSON body for creating a subscriber.
type apiSubscriberCreate struct {
	Subscriber apiSubscriberBody `json:"subscriber"`
}

// apiSubscriberBody holds the subscriber fields.
type apiSubscriberBody struct {
	Email        string   `json:"email,omitempty"`
	Endpoint     string   `json:"endpoint,omitempty"`
	ComponentIDs []string `json:"component_ids,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID           types.String `tfsdk:"id"`
	PageID       types.String `tfsdk:"page_id"`
	Email        types.String `tfsdk:"email"`
	Endpoint     types.String `tfsdk:"endpoint"`
	ComponentIDs types.List   `tfsdk:"component_ids"`
}

// Resource implements the atlassian_statuspage_subscriber managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_subscriber"
}

// Schema defines the schema for the subscriber resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Statuspage subscriber.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the subscriber.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"email": schema.StringAttribute{
				Description: "The email address of the subscriber.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint": schema.StringAttribute{
				Description: "The webhook endpoint URL for the subscriber.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"component_ids": schema.ListAttribute{
				Description: "The list of component IDs to subscribe to.",
				Optional:    true,
				ElementType: types.StringType,
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

// Create provisions a new subscriber.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiSubscriberCreate{
		Subscriber: apiSubscriberBody{},
	}
	if !plan.Email.IsNull() && !plan.Email.IsUnknown() {
		body.Subscriber.Email = plan.Email.ValueString()
	}
	if !plan.Endpoint.IsNull() && !plan.Endpoint.IsUnknown() {
		body.Subscriber.Endpoint = plan.Endpoint.ValueString()
	}
	if !plan.ComponentIDs.IsNull() && !plan.ComponentIDs.IsUnknown() {
		var ids []string
		resp.Diagnostics.Append(plan.ComponentIDs.ElementsAs(ctx, &ids, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body.Subscriber.ComponentIDs = ids
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiSubscriber
	err := r.client.Post(ctx, fmt.Sprintf("/v1/pages/%s/subscribers", plan.PageID.ValueString()), bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusForbidden {
			resp.Diagnostics.AddError(
				"Permission denied",
				"The authenticated user does not have permission to create Statuspage subscribers. Ensure the service account has Statuspage admin privileges.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to create Statuspage subscriber",
			fmt.Sprintf("Could not create subscriber on page %q: %s",
				plan.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.PageID = types.StringValue(created.PageID)
	plan.Email = types.StringValue(created.Email)
	plan.Endpoint = types.StringValue(created.Endpoint)
	compIDs, diags := types.ListValueFrom(ctx, types.StringType, created.ComponentIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ComponentIDs = compIDs

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var sub apiSubscriber
	err := r.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/subscribers/%s", state.PageID.ValueString(), state.ID.ValueString()), &sub)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage subscriber",
			fmt.Sprintf("Could not read subscriber %q on page %q: %s. Verify the subscriber exists and has not been removed.",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(sub.ID)
	state.PageID = types.StringValue(sub.PageID)
	state.Email = types.StringValue(sub.Email)
	state.Endpoint = types.StringValue(sub.Endpoint)
	compIDs, diags := types.ListValueFrom(ctx, types.StringType, sub.ComponentIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.ComponentIDs = compIDs

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing subscriber.
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

	body := apiSubscriberCreate{
		Subscriber: apiSubscriberBody{},
	}
	if !plan.Email.IsNull() && !plan.Email.IsUnknown() {
		body.Subscriber.Email = plan.Email.ValueString()
	}
	if !plan.Endpoint.IsNull() && !plan.Endpoint.IsUnknown() {
		body.Subscriber.Endpoint = plan.Endpoint.ValueString()
	}
	if !plan.ComponentIDs.IsNull() && !plan.ComponentIDs.IsUnknown() {
		var ids []string
		resp.Diagnostics.Append(plan.ComponentIDs.ElementsAs(ctx, &ids, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body.Subscriber.ComponentIDs = ids
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiSubscriber
	err := r.client.Put(ctx, fmt.Sprintf("/v1/pages/%s/subscribers/%s", state.PageID.ValueString(), state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Subscriber not found",
					fmt.Sprintf("Subscriber %q on page %q not found. The subscriber may have been removed outside of Terraform.",
						state.ID.ValueString(), state.PageID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update Statuspage subscribers. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Statuspage subscriber",
			fmt.Sprintf("Could not update subscriber %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.PageID = types.StringValue(updated.PageID)
	plan.Email = types.StringValue(updated.Email)
	plan.Endpoint = types.StringValue(updated.Endpoint)
	compIDs, diags := types.ListValueFrom(ctx, types.StringType, updated.ComponentIDs)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ComponentIDs = compIDs

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a subscriber.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/v1/pages/%s/subscribers/%s", state.PageID.ValueString(), state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete subscriber %q. "+
						"Ensure the service account has Statuspage admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Statuspage subscriber",
			fmt.Sprintf("Could not delete subscriber %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing subscriber by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
