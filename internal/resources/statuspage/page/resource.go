// Package page implements the atlassian_statuspage_page managed resource.
//
// This resource manages Statuspage pages through the Atlassian
// Statuspage REST API (v1). It supports full CRUD operations and
// state import via page ID.
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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiPage represents the JSON structure returned by the Statuspage page API.
type apiPage struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	PageDescription string `json:"page_description"`
	Subdomain       string `json:"subdomain"`
	URL             string `json:"url"`
}

// apiPageCreate represents the JSON body for creating a Statuspage page.
type apiPageCreate struct {
	Page apiPageCreateBody `json:"page"`
}

// apiPageCreateBody holds the page creation fields.
type apiPageCreateBody struct {
	Name            string `json:"name"`
	PageDescription string `json:"page_description,omitempty"`
	Subdomain       string `json:"subdomain,omitempty"`
}

// apiPageUpdate represents the JSON body for updating a Statuspage page.
type apiPageUpdate struct {
	Page apiPageUpdateBody `json:"page"`
}

// apiPageUpdateBody holds the page update fields.
type apiPageUpdateBody struct {
	Name            string `json:"name,omitempty"`
	PageDescription string `json:"page_description,omitempty"`
	Subdomain       string `json:"subdomain,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	PageDescription types.String `tfsdk:"page_description"`
	Subdomain       types.String `tfsdk:"subdomain"`
	URL             types.String `tfsdk:"url"`
}

// Resource implements the atlassian_statuspage_page managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_page"
}

// Schema defines the schema for the Statuspage page resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Statuspage page in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the page, assigned by Statuspage.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Statuspage page.",
				Required:    true,
			},
			"page_description": schema.StringAttribute{
				Description: "A description of the Statuspage page.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"subdomain": schema.StringAttribute{
				Description: "The subdomain for the Statuspage page.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"url": schema.StringAttribute{
				Description: "The URL of the Statuspage page.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
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

// Create provisions a new Statuspage page.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiPageCreate{
		Page: apiPageCreateBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.PageDescription.IsNull() && !plan.PageDescription.IsUnknown() {
		body.Page.PageDescription = plan.PageDescription.ValueString()
	}
	if !plan.Subdomain.IsNull() && !plan.Subdomain.IsUnknown() {
		body.Page.Subdomain = plan.Subdomain.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiPage
	err := r.client.Post(ctx, "/v1/pages", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate Statuspage page",
					fmt.Sprintf("A Statuspage page with name %q already exists. Each page name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create Statuspage pages. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create Statuspage page",
			fmt.Sprintf("Could not create Statuspage page with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.PageDescription = types.StringValue(created.PageDescription)
	plan.Subdomain = types.StringValue(created.Subdomain)
	plan.URL = types.StringValue(created.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Statuspage.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var page apiPage
	err := r.client.Get(ctx, fmt.Sprintf("/v1/pages/%s", state.ID.ValueString()), &page)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage page",
			fmt.Sprintf("Could not read Statuspage page %q: %s. Verify the page exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(page.ID)
	state.Name = types.StringValue(page.Name)
	state.PageDescription = types.StringValue(page.PageDescription)
	state.Subdomain = types.StringValue(page.Subdomain)
	state.URL = types.StringValue(page.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Statuspage page.
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
		Page: apiPageUpdateBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.PageDescription.IsNull() && !plan.PageDescription.IsUnknown() {
		body.Page.PageDescription = plan.PageDescription.ValueString()
	}
	if !plan.Subdomain.IsNull() && !plan.Subdomain.IsUnknown() {
		body.Page.Subdomain = plan.Subdomain.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiPage
	err := r.client.Put(ctx, fmt.Sprintf("/v1/pages/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Statuspage page not found",
					fmt.Sprintf("Statuspage page with ID %q not found. The page may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update Statuspage pages. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Statuspage page",
			fmt.Sprintf("Could not update Statuspage page with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.PageDescription = types.StringValue(updated.PageDescription)
	plan.Subdomain = types.StringValue(updated.Subdomain)
	plan.URL = types.StringValue(updated.URL)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Statuspage page.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/v1/pages/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete Statuspage page %q. "+
						"Ensure the service account has Statuspage admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Statuspage page",
			fmt.Sprintf("Could not delete Statuspage page with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing Statuspage page by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
