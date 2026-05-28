// Package page also implements the atlassian_confluence_content_restriction managed resource.
//
// This resource manages content restrictions on Confluence pages through the
// Atlassian Cloud REST API (v2). It supports full CRUD operations and state
// import via composite ID.
package page

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

// Ensure the RestrictionResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &RestrictionResource{}
	_ resource.ResourceWithImportState = &RestrictionResource{}
)

// apiRestriction represents the JSON structure for a content restriction.
type apiRestriction struct {
	ID            string                   `json:"id,omitempty"`
	Operation     string                   `json:"operation"`
	Restrictions  apiRestrictionPrincipals `json:"restrictions,omitempty"`
	ContentID     string                   `json:"-"`
	PrincipalType string                   `json:"-"`
	PrincipalID   string                   `json:"-"`
}

// apiRestrictionPrincipals holds user and group restrictions.
type apiRestrictionPrincipals struct {
	User  apiRestrictionList `json:"user,omitempty"`
	Group apiRestrictionList `json:"group,omitempty"`
}

// apiRestrictionList holds a list of restriction entries.
type apiRestrictionList struct {
	Results []apiRestrictionEntry `json:"results,omitempty"`
}

// apiRestrictionEntry represents a single restriction entry.
type apiRestrictionEntry struct {
	Type      string `json:"type"`
	AccountID string `json:"accountId,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
}

// apiRestrictionCreate represents the JSON body for creating a restriction.
type apiRestrictionCreate struct {
	Operation    string                         `json:"operation"`
	Restrictions apiRestrictionCreatePrincipals `json:"restrictions"`
}

// apiRestrictionCreatePrincipals holds user and group principals for creation.
type apiRestrictionCreatePrincipals struct {
	User  []apiRestrictionEntry `json:"user,omitempty"`
	Group []apiRestrictionEntry `json:"group,omitempty"`
}

// RestrictionResourceModel describes the resource data model.
type RestrictionResourceModel struct {
	ID            types.String `tfsdk:"id"`
	ContentID     types.String `tfsdk:"content_id"`
	Operation     types.String `tfsdk:"operation"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
}

// RestrictionResource implements the atlassian_confluence_content_restriction managed resource.
type RestrictionResource struct {
	client *atlassian.Client
}

// NewRestrictionResource returns a new RestrictionResource instance for provider registration.
func NewRestrictionResource() resource.Resource {
	return &RestrictionResource{}
}

// Metadata returns the resource type name.
func (r *RestrictionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_content_restriction"
}

// Schema defines the schema for the content restriction resource.
func (r *RestrictionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a content restriction on a Confluence page in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The composite identifier of the restriction (content_id/operation/principal_type/principal_id).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"content_id": schema.StringAttribute{
				Description: "The ID of the content (page) to restrict.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"operation": schema.StringAttribute{
				Description: "The operation to restrict. Must be \"read\" or \"update\".",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal. Must be \"user\" or \"group\".",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"principal_id": schema.StringAttribute{
				Description: "The ID of the principal (user account ID or group ID).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

// Configure sets the provider-configured client on the resource.
func (r *RestrictionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// compositeRestrictionID builds a composite ID from the restriction attributes.
func compositeRestrictionID(contentID, operation, principalType, principalID string) string {
	return fmt.Sprintf("%s/%s/%s/%s", contentID, operation, principalType, principalID)
}

// parseRestrictionID parses a composite restriction ID into components.
func parseRestrictionID(id string) (contentID, operation, principalType, principalID string, err error) {
	parts := strings.SplitN(id, "/", 4)
	if len(parts) != 4 {
		return "", "", "", "", fmt.Errorf("expected format: content_id/operation/principal_type/principal_id, got %q", id)
	}
	return parts[0], parts[1], parts[2], parts[3], nil
}

// Create provisions a new content restriction.
func (r *RestrictionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan RestrictionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiRestrictionCreate{
		Operation: plan.Operation.ValueString(),
	}
	entry := apiRestrictionEntry{}
	if plan.PrincipalType.ValueString() == "user" {
		entry.Type = "known"
		entry.AccountID = plan.PrincipalID.ValueString()
		body.Restrictions.User = []apiRestrictionEntry{entry}
	} else {
		entry.Type = "group"
		entry.ID = plan.PrincipalID.ValueString()
		body.Restrictions.Group = []apiRestrictionEntry{entry}
	}
	bodyBytes, _ := json.Marshal([]apiRestrictionCreate{body})

	err := r.client.Put(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restriction", plan.ContentID.ValueString()), bytes.NewReader(bodyBytes), nil)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to manage content restrictions on Confluence page %q. Ensure the service account has Confluence admin privileges.",
						plan.ContentID.ValueString()),
				)
				return
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Confluence page not found",
					fmt.Sprintf("Confluence page with ID %q not found. Verify the page exists and has not been deleted.", plan.ContentID.ValueString()),
				)
				return
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Confluence content restriction conflict",
					fmt.Sprintf("A conflicting restriction already exists on Confluence page %q for operation %q. Remove the existing restriction first or import it into Terraform.",
						plan.ContentID.ValueString(), plan.Operation.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create Confluence content restriction",
			fmt.Sprintf("Could not create restriction on Confluence page %q: %s", plan.ContentID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(compositeRestrictionID(
		plan.ContentID.ValueString(),
		plan.Operation.ValueString(),
		plan.PrincipalType.ValueString(),
		plan.PrincipalID.ValueString(),
	))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *RestrictionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state RestrictionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var restrictions []apiRestriction
	err := r.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restriction", state.ContentID.ValueString()), &restrictions)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Confluence content restrictions",
			fmt.Sprintf("Could not read restrictions for Confluence page %q: %s. Verify the page exists and has not been deleted.", state.ContentID.ValueString(), err.Error()),
		)
		return
	}

	found := false
	for _, restriction := range restrictions {
		if restriction.Operation != state.Operation.ValueString() {
			continue
		}
		if state.PrincipalType.ValueString() == "user" {
			for _, u := range restriction.Restrictions.User.Results {
				if u.AccountID == state.PrincipalID.ValueString() {
					found = true
					break
				}
			}
		} else {
			for _, g := range restriction.Restrictions.Group.Results {
				if g.ID == state.PrincipalID.ValueString() {
					found = true
					break
				}
			}
		}
	}

	if !found {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing content restriction (replace-in-place via delete+create).
func (r *RestrictionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Content restrictions are immutable. Changes require replacement.",
	)
}

// Delete removes a content restriction.
func (r *RestrictionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state RestrictionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restriction/%s/%s/%s",
		state.ContentID.ValueString(),
		state.Operation.ValueString(),
		state.PrincipalType.ValueString(),
		state.PrincipalID.ValueString(),
	))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete restriction on Confluence page %q. Ensure the service account has Confluence admin privileges.", state.ContentID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Confluence content restriction",
			fmt.Sprintf("Could not delete restriction on Confluence page %q: %s", state.ContentID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing content restriction by composite ID.
func (r *RestrictionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	contentID, operation, principalType, principalID, err := parseRestrictionID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected format: content_id/operation/principal_type/principal_id. Error: %s", err.Error()),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("content_id"), contentID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("operation"), operation)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("principal_type"), principalType)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("principal_id"), principalID)...)
}
