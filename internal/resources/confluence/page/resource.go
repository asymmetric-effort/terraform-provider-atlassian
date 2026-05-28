// Package page implements the atlassian_confluence_page managed resource.
//
// This resource manages Confluence pages through the Atlassian Cloud REST API
// (v2). It supports full CRUD operations and state import via page ID.
package page

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiPage represents the JSON structure returned by the Confluence page API.
type apiPage struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	SpaceID  string         `json:"spaceId"`
	ParentID string         `json:"parentId,omitempty"`
	Status   string         `json:"status"`
	Body     apiPageBody    `json:"body,omitempty"`
	Version  apiPageVersion `json:"version,omitempty"`
}

// apiPageBody represents the body content of a Confluence page.
type apiPageBody struct {
	Storage apiPageStorage `json:"storage,omitempty"`
}

// apiPageStorage represents the storage-format body content.
type apiPageStorage struct {
	Value          string `json:"value,omitempty"`
	Representation string `json:"representation,omitempty"`
}

// apiPageVersion represents the version information of a page.
type apiPageVersion struct {
	Number int64 `json:"number"`
}

// apiPageCreate represents the JSON body for creating a page.
type apiPageCreate struct {
	SpaceID  string      `json:"spaceId"`
	Title    string      `json:"title"`
	ParentID string      `json:"parentId,omitempty"`
	Status   string      `json:"status"`
	Body     apiPageBody `json:"body,omitempty"`
}

// apiPageUpdate represents the JSON body for updating a page.
type apiPageUpdate struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	SpaceID string         `json:"spaceId"`
	Status  string         `json:"status"`
	Body    apiPageBody    `json:"body,omitempty"`
	Version apiPageVersion `json:"version"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID       types.String `tfsdk:"id"`
	SpaceID  types.String `tfsdk:"space_id"`
	Title    types.String `tfsdk:"title"`
	Body     types.String `tfsdk:"body"`
	ParentID types.String `tfsdk:"parent_id"`
	Status   types.String `tfsdk:"status"`
	Version  types.Int64  `tfsdk:"version"`
}

// Resource implements the atlassian_confluence_page managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_page"
}

// Schema defines the schema for the confluence page resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Confluence page in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the page, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the space containing the page.",
				Required:    true,
			},
			"title": schema.StringAttribute{
				Description: "The title of the page.",
				Required:    true,
			},
			"body": schema.StringAttribute{
				Description: "The body content of the page in storage format.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"parent_id": schema.StringAttribute{
				Description: "The ID of the parent page. If omitted, the page is created at the top level.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "The status of the page (e.g., current, draft).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.Int64Attribute{
				Description: "The current version number of the page.",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
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

// Create provisions a new Confluence page.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiPageCreate{
		SpaceID: plan.SpaceID.ValueString(),
		Title:   plan.Title.ValueString(),
		Status:  "current",
	}
	if !plan.ParentID.IsNull() && !plan.ParentID.IsUnknown() {
		body.ParentID = plan.ParentID.ValueString()
	}
	if !plan.Body.IsNull() && !plan.Body.IsUnknown() {
		body.Body = apiPageBody{
			Storage: apiPageStorage{
				Value:          plan.Body.ValueString(),
				Representation: "storage",
			},
		}
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiPage
	err := r.client.Post(ctx, "/wiki/api/v2/pages", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate Confluence page",
					fmt.Sprintf("A Confluence page with title %q already exists in space %q. Use a unique title or import the existing page.", plan.Title.ValueString(), plan.SpaceID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create Confluence pages. Ensure the service account has Confluence admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create Confluence page",
			fmt.Sprintf("Could not create Confluence page %q in space %q: %s", plan.Title.ValueString(), plan.SpaceID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.SpaceID = types.StringValue(created.SpaceID)
	plan.Title = types.StringValue(created.Title)
	plan.Body = types.StringValue(created.Body.Storage.Value)
	plan.ParentID = types.StringValue(created.ParentID)
	plan.Status = types.StringValue(created.Status)
	plan.Version = types.Int64Value(created.Version.Number)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var page apiPage
	err := r.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s?body-format=storage", state.ID.ValueString()), &page)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Confluence page",
			fmt.Sprintf("Could not read Confluence page with ID %q: %s. Verify the page exists and has not been deleted.", state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(page.ID)
	state.SpaceID = types.StringValue(page.SpaceID)
	state.Title = types.StringValue(page.Title)
	state.Body = types.StringValue(page.Body.Storage.Value)
	state.ParentID = types.StringValue(page.ParentID)
	state.Status = types.StringValue(page.Status)
	state.Version = types.Int64Value(page.Version.Number)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Confluence page.
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

	body := apiPageUpdate{
		ID:      state.ID.ValueString(),
		Title:   plan.Title.ValueString(),
		SpaceID: plan.SpaceID.ValueString(),
		Status:  "current",
		Version: apiPageVersion{Number: state.Version.ValueInt64() + 1},
	}
	if !plan.Body.IsNull() && !plan.Body.IsUnknown() {
		body.Body = apiPageBody{
			Storage: apiPageStorage{
				Value:          plan.Body.ValueString(),
				Representation: "storage",
			},
		}
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiPage
	err := r.client.Put(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Confluence page not found",
					fmt.Sprintf("Confluence page with ID %q not found. The page may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to update Confluence page %q. Ensure the service account has Confluence admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Confluence page",
			fmt.Sprintf("Could not update Confluence page with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.SpaceID = types.StringValue(updated.SpaceID)
	plan.Title = types.StringValue(updated.Title)
	plan.Body = types.StringValue(updated.Body.Storage.Value)
	plan.ParentID = types.StringValue(updated.ParentID)
	plan.Status = types.StringValue(updated.Status)
	plan.Version = types.Int64Value(updated.Version.Number)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Confluence page.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete Confluence page %q. Ensure the service account has Confluence admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Confluence page",
			fmt.Sprintf("Could not delete Confluence page with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing page by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
