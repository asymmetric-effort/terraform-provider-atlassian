// Package template implements the atlassian_confluence_template managed resource.
//
// This resource manages Confluence templates through the Atlassian Cloud REST
// API (v2). It supports full CRUD operations and state import via template ID.
package template

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

// apiTemplate represents the JSON structure returned by the Confluence template API.
type apiTemplate struct {
	TemplateID   string `json:"templateId"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Body         string `json:"body,omitempty"`
	SpaceID      string `json:"spaceId,omitempty"`
	TemplateType string `json:"templateType,omitempty"`
}

// apiTemplateCreate represents the JSON body for creating a template.
type apiTemplateCreate struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Body         string `json:"body,omitempty"`
	SpaceID      string `json:"spaceId,omitempty"`
	TemplateType string `json:"templateType,omitempty"`
}

// apiTemplateUpdate represents the JSON body for updating a template.
type apiTemplateUpdate struct {
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	Body         string `json:"body,omitempty"`
	SpaceID      string `json:"spaceId,omitempty"`
	TemplateType string `json:"templateType,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Body         types.String `tfsdk:"body"`
	SpaceID      types.String `tfsdk:"space_id"`
	TemplateType types.String `tfsdk:"template_type"`
}

// Resource implements the atlassian_confluence_template managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_template"
}

// Schema defines the schema for the confluence template resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Confluence template in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the template, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the template.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the template.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"body": schema.StringAttribute{
				Description: "The body content of the template in storage format.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the space for a space-scoped template. If omitted, the template is global.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"template_type": schema.StringAttribute{
				Description: "The type of template (e.g., page).",
				Optional:    true,
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

// Create provisions a new Confluence template.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiTemplateCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.Body.IsNull() && !plan.Body.IsUnknown() {
		body.Body = plan.Body.ValueString()
	}
	if !plan.SpaceID.IsNull() && !plan.SpaceID.IsUnknown() {
		body.SpaceID = plan.SpaceID.ValueString()
	}
	if !plan.TemplateType.IsNull() && !plan.TemplateType.IsUnknown() {
		body.TemplateType = plan.TemplateType.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiTemplate
	err := r.client.Post(ctx, "/wiki/api/v2/templates", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate template",
					fmt.Sprintf("A template with name %q already exists.", plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create templates.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create template",
			fmt.Sprintf("Could not create template %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.TemplateID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)
	plan.Body = types.StringValue(created.Body)
	plan.SpaceID = types.StringValue(created.SpaceID)
	plan.TemplateType = types.StringValue(created.TemplateType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tmpl apiTemplate
	err := r.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", state.ID.ValueString()), &tmpl)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read template",
			fmt.Sprintf("Could not read template %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(tmpl.TemplateID)
	state.Name = types.StringValue(tmpl.Name)
	state.Description = types.StringValue(tmpl.Description)
	state.Body = types.StringValue(tmpl.Body)
	state.SpaceID = types.StringValue(tmpl.SpaceID)
	state.TemplateType = types.StringValue(tmpl.TemplateType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Confluence template.
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

	body := apiTemplateUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	if !plan.Body.IsNull() && !plan.Body.IsUnknown() {
		body.Body = plan.Body.ValueString()
	}
	if !plan.SpaceID.IsNull() && !plan.SpaceID.IsUnknown() {
		body.SpaceID = plan.SpaceID.ValueString()
	}
	if !plan.TemplateType.IsNull() && !plan.TemplateType.IsUnknown() {
		body.TemplateType = plan.TemplateType.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiTemplate
	err := r.client.Put(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Template not found",
					fmt.Sprintf("Template %q not found. It may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update templates.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update template",
			fmt.Sprintf("Could not update template %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.TemplateID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)
	plan.Body = types.StringValue(updated.Body)
	plan.SpaceID = types.StringValue(updated.SpaceID)
	plan.TemplateType = types.StringValue(updated.TemplateType)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Confluence template.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete template %q.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete template",
			fmt.Sprintf("Could not delete template %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing template by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
