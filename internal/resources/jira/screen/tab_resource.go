// Package screen implements the atlassian_jira_screen_tab managed resource.
//
// This resource manages Jira screen tabs through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via composite ID (screenId/tabId).
package screen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the TabResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &TabResource{}
	_ resource.ResourceWithImportState = &TabResource{}
)

// apiTab represents the JSON structure returned by the Atlassian screen tab API.
type apiTab struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position,omitempty"`
}

// apiTabCreate represents the JSON body for creating a screen tab.
type apiTabCreate struct {
	Name string `json:"name"`
}

// apiTabUpdate represents the JSON body for updating a screen tab.
type apiTabUpdate struct {
	Name string `json:"name,omitempty"`
}

// TabResourceModel describes the resource data model for screen tabs.
type TabResourceModel struct {
	ID       types.String `tfsdk:"id"`
	ScreenID types.String `tfsdk:"screen_id"`
	Name     types.String `tfsdk:"name"`
	Position types.Int64  `tfsdk:"position"`
}

// TabResource implements the atlassian_jira_screen_tab managed resource.
type TabResource struct {
	client *atlassian.Client
}

// NewTabResource returns a new TabResource instance for provider registration.
func NewTabResource() resource.Resource {
	return &TabResource{}
}

// Metadata returns the resource type name.
func (r *TabResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_tab"
}

// Schema defines the schema for the jira screen tab resource.
func (r *TabResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a tab on a Jira screen in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The composite identifier (screenId/tabId).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"screen_id": schema.StringAttribute{
				Description: "The ID of the screen that owns this tab.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The display name of the screen tab.",
				Required:    true,
			},
			"position": schema.Int64Attribute{
				Description: "The position of the tab on the screen, used for ordering.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *TabResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create adds a new tab to a Jira screen.
func (r *TabResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TabResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiTabCreate{
		Name: plan.Name.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs", plan.ScreenID.ValueString())

	var created apiTab
	err := r.client.Post(ctx, endpoint, bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Screen not found",
					fmt.Sprintf("Screen %q not found. Verify the screen ID is correct and the screen exists.",
						plan.ScreenID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid screen tab configuration",
					fmt.Sprintf("Could not create tab %q on screen %s. Verify the tab name is valid.",
						plan.Name.ValueString(), plan.ScreenID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create screen tabs. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create screen tab",
			fmt.Sprintf("Could not create tab %q on screen %s: %s",
				plan.Name.ValueString(), plan.ScreenID.ValueString(), err.Error()),
		)
		return
	}

	tabID := fmt.Sprintf("%d", created.ID)
	compositeID := fmt.Sprintf("%s/%s", plan.ScreenID.ValueString(), tabID)
	plan.ID = types.StringValue(compositeID)
	plan.Name = types.StringValue(created.Name)

	// Save the originally requested position before overwriting with server value
	requestedPosition := plan.Position
	plan.Position = types.Int64Value(int64(created.Position))

	// Move tab to requested position if specified
	if !requestedPosition.IsNull() && !requestedPosition.IsUnknown() {
		requestedPos := requestedPosition.ValueInt64()
		if int64(created.Position) != requestedPos {
			moveEndpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/move/%d",
				plan.ScreenID.ValueString(), tabID, requestedPos)
			moveErr := r.client.Post(ctx, moveEndpoint, nil, nil)
			if moveErr != nil {
				resp.Diagnostics.AddError(
					"Failed to move screen tab",
					fmt.Sprintf("Tab was created but could not be moved to position %d: %s",
						requestedPos, moveErr.Error()),
				)
				return
			}
			plan.Position = types.Int64Value(requestedPos)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *TabResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TabResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Extract tabId from composite ID
	parts := strings.SplitN(state.ID.ValueString(), "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid resource ID",
			fmt.Sprintf("Expected composite ID format screenId/tabId, got: %q", state.ID.ValueString()),
		)
		return
	}
	tabID := parts[1]

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs", state.ScreenID.ValueString())

	var tabs []apiTab
	err := r.client.Get(ctx, endpoint, &tabs)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read screen tabs",
			fmt.Sprintf("Could not read tabs for screen %s: %s",
				state.ScreenID.ValueString(), err.Error()),
		)
		return
	}

	// Find the specific tab in the list
	found := false
	for _, tab := range tabs {
		if fmt.Sprintf("%d", tab.ID) == tabID {
			found = true
			state.Name = types.StringValue(tab.Name)
			state.Position = types.Int64Value(int64(tab.Position))
			break
		}
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira screen tab.
func (r *TabResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TabResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state TabResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Extract tabId from composite ID
	parts := strings.SplitN(state.ID.ValueString(), "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid resource ID",
			fmt.Sprintf("Expected composite ID format screenId/tabId, got: %q", state.ID.ValueString()),
		)
		return
	}
	tabID := parts[1]

	body := apiTabUpdate{
		Name: plan.Name.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s",
		state.ScreenID.ValueString(), tabID)

	var updated apiTab
	err := r.client.Put(ctx, endpoint, bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Screen tab not found",
					fmt.Sprintf("Screen %q tab %q not found. The tab may have been deleted outside of Terraform.",
						state.ScreenID.ValueString(), tabID),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid screen tab configuration",
					fmt.Sprintf("The tab update for screen %q tab %q is invalid. Verify the tab name is valid.",
						state.ScreenID.ValueString(), tabID),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update screen tabs. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update screen tab",
			fmt.Sprintf("Could not update tab %q on screen %s: %s",
				tabID, state.ScreenID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = state.ID
	plan.ScreenID = state.ScreenID
	plan.Name = types.StringValue(updated.Name)

	// Save the originally requested position before overwriting with server value
	requestedPosition := plan.Position
	plan.Position = types.Int64Value(int64(updated.Position))

	// Move tab to requested position if it changed
	if !requestedPosition.IsNull() && !requestedPosition.IsUnknown() {
		requestedPos := requestedPosition.ValueInt64()
		if int64(updated.Position) != requestedPos {
			moveEndpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/move/%d",
				state.ScreenID.ValueString(), tabID, requestedPos)
			moveErr := r.client.Post(ctx, moveEndpoint, nil, nil)
			if moveErr != nil {
				resp.Diagnostics.AddError(
					"Failed to move screen tab",
					fmt.Sprintf("Tab was updated but could not be moved to position %d: %s",
						requestedPos, moveErr.Error()),
				)
				return
			}
			plan.Position = types.Int64Value(requestedPos)
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a tab from a Jira screen.
func (r *TabResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TabResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Extract tabId from composite ID
	parts := strings.SplitN(state.ID.ValueString(), "/", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid resource ID",
			fmt.Sprintf("Expected composite ID format screenId/tabId, got: %q", state.ID.ValueString()),
		)
		return
	}
	tabID := parts[1]

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s",
		state.ScreenID.ValueString(), tabID)

	err := r.client.Delete(ctx, endpoint)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete tab %q from screen %q. "+
						"Ensure the service account has Jira admin privileges.", tabID, state.ScreenID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete screen tab",
			fmt.Sprintf("Could not delete tab %q from screen %s: %s",
				tabID, state.ScreenID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing screen tab by composite ID (screenId/tabId).
func (r *TabResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID format: screenId/tabId, got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("screen_id"), parts[0])...)
}
