// Package screen implements the atlassian_jira_screen_tab_field managed resource.
//
// This resource manages Jira screen tab fields through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via composite ID (screenId/tabId/fieldId).
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the TabFieldResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &TabFieldResource{}
	_ resource.ResourceWithImportState = &TabFieldResource{}
)

// apiTabField represents the JSON structure returned by the Atlassian screen tab field API.
type apiTabField struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// apiTabFieldCreate represents the JSON body for adding a field to a screen tab.
type apiTabFieldCreate struct {
	FieldID string `json:"fieldId"`
}

// TabFieldResourceModel describes the resource data model for screen tab fields.
type TabFieldResourceModel struct {
	ID       types.String `tfsdk:"id"`
	ScreenID types.String `tfsdk:"screen_id"`
	TabID    types.String `tfsdk:"tab_id"`
	FieldID  types.String `tfsdk:"field_id"`
}

// TabFieldResource implements the atlassian_jira_screen_tab_field managed resource.
type TabFieldResource struct {
	client *atlassian.Client
}

// NewTabFieldResource returns a new TabFieldResource instance for provider registration.
func NewTabFieldResource() resource.Resource {
	return &TabFieldResource{}
}

// Metadata returns the resource type name.
func (r *TabFieldResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_screen_tab_field"
}

// Schema defines the schema for the jira screen tab field resource.
func (r *TabFieldResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a field on a Jira screen tab in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The composite identifier (screenId/tabId/fieldId).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"screen_id": schema.StringAttribute{
				Description: "The ID of the screen.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tab_id": schema.StringAttribute{
				Description: "The ID of the screen tab.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"field_id": schema.StringAttribute{
				Description: "The ID of the field to add to the screen tab.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *TabFieldResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create adds a field to a Jira screen tab.
func (r *TabFieldResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan TabFieldResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiTabFieldCreate{
		FieldID: plan.FieldID.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/fields",
		plan.ScreenID.ValueString(), plan.TabID.ValueString())

	var created apiTabField
	err := r.client.Post(ctx, endpoint, bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Screen or tab not found",
					fmt.Sprintf("Screen %q or tab %q not found. Verify the screen and tab IDs are correct and the resources exist.",
						plan.ScreenID.ValueString(), plan.TabID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid screen tab field configuration",
					fmt.Sprintf("Could not add field %q to screen %s tab %s. Verify the field ID is valid and not already present on the tab.",
						plan.FieldID.ValueString(), plan.ScreenID.ValueString(), plan.TabID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to add fields to screen tabs. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to add field to screen tab",
			fmt.Sprintf("Could not add field %q to screen %s tab %s: %s",
				plan.FieldID.ValueString(), plan.ScreenID.ValueString(), plan.TabID.ValueString(), err.Error()),
		)
		return
	}

	compositeID := fmt.Sprintf("%s/%s/%s", plan.ScreenID.ValueString(), plan.TabID.ValueString(), plan.FieldID.ValueString())
	plan.ID = types.StringValue(compositeID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *TabFieldResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state TabFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/fields",
		state.ScreenID.ValueString(), state.TabID.ValueString())

	var fields []apiTabField
	err := r.client.Get(ctx, endpoint, &fields)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read screen tab fields",
			fmt.Sprintf("Could not read fields for screen %s tab %s: %s",
				state.ScreenID.ValueString(), state.TabID.ValueString(), err.Error()),
		)
		return
	}

	// Find the specific field in the list
	found := false
	for _, f := range fields {
		if f.ID == state.FieldID.ValueString() {
			found = true
			break
		}
	}
	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	// State is already correct, no updates needed.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is a no-op for screen tab fields since all attributes require replacement.
func (r *TabFieldResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Unexpected update",
		"Screen tab field attributes all require replacement. Update should not be called.",
	)
}

// Delete removes a field from a Jira screen tab.
func (r *TabFieldResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state TabFieldResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := fmt.Sprintf("/rest/api/3/screens/%s/tabs/%s/fields/%s",
		state.ScreenID.ValueString(), state.TabID.ValueString(), state.FieldID.ValueString())

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
					fmt.Sprintf("The authenticated user does not have permission to remove field %q from screen tab. "+
						"Ensure the service account has Jira admin privileges.", state.FieldID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to remove field from screen tab",
			fmt.Sprintf("Could not remove field %q from screen %s tab %s: %s",
				state.FieldID.ValueString(), state.ScreenID.ValueString(), state.TabID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing screen tab field by composite ID (screenId/tabId/fieldId).
func (r *TabFieldResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected import ID format: screenId/tabId/fieldId, got: %q", req.ID),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("screen_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tab_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("field_id"), parts[2])...)
}
