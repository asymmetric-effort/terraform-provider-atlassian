// Package page implements the atlassian_statuspage_incident_template managed resource.
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

// Ensure the IncidentTemplateResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &IncidentTemplateResource{}
	_ resource.ResourceWithImportState = &IncidentTemplateResource{}
)

// apiIncidentTemplate represents the JSON structure returned by the incident template API.
type apiIncidentTemplate struct {
	ID     string `json:"id"`
	PageID string `json:"page_id"`
	Name   string `json:"name"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// apiIncidentTemplateCreate represents the JSON body for creating an incident template.
type apiIncidentTemplateCreate struct {
	Template apiIncidentTemplateBody `json:"template"`
}

// apiIncidentTemplateBody holds the template fields.
type apiIncidentTemplateBody struct {
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

// IncidentTemplateResourceModel describes the resource data model.
type IncidentTemplateResourceModel struct {
	ID     types.String `tfsdk:"id"`
	PageID types.String `tfsdk:"page_id"`
	Name   types.String `tfsdk:"name"`
	Title  types.String `tfsdk:"title"`
	Body   types.String `tfsdk:"body"`
}

// IncidentTemplateResource implements the atlassian_statuspage_incident_template managed resource.
type IncidentTemplateResource struct {
	client *atlassian.Client
}

// NewIncidentTemplateResource returns a new IncidentTemplateResource instance for provider registration.
func NewIncidentTemplateResource() resource.Resource {
	return &IncidentTemplateResource{}
}

// Metadata returns the resource type name.
func (r *IncidentTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_incident_template"
}

// Schema defines the schema for the incident template resource.
func (r *IncidentTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Statuspage incident template.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the incident template.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"page_id": schema.StringAttribute{
				Description: "The ID of the Statuspage page this template belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the incident template.",
				Required:    true,
			},
			"title": schema.StringAttribute{
				Description: "The default title for incidents created from this template.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"body": schema.StringAttribute{
				Description: "The default body for incidents created from this template.",
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
func (r *IncidentTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new incident template.
func (r *IncidentTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan IncidentTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIncidentTemplateCreate{
		Template: apiIncidentTemplateBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.Title.IsNull() && !plan.Title.IsUnknown() {
		body.Template.Title = plan.Title.ValueString()
	}
	if !plan.Body.IsNull() && !plan.Body.IsUnknown() {
		body.Template.Body = plan.Body.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiIncidentTemplate
	err := r.client.Post(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates", plan.PageID.ValueString()), bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusForbidden {
			resp.Diagnostics.AddError(
				"Permission denied",
				"The authenticated user does not have permission to create incident templates. Ensure the service account has Statuspage admin privileges.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to create Statuspage incident template",
			fmt.Sprintf("Could not create incident template with name %q on page %q: %s",
				plan.Name.ValueString(), plan.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.PageID = types.StringValue(created.PageID)
	plan.Name = types.StringValue(created.Name)
	plan.Title = types.StringValue(created.Title)
	plan.Body = types.StringValue(created.Body)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data.
func (r *IncidentTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state IncidentTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tmpl apiIncidentTemplate
	err := r.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates/%s", state.PageID.ValueString(), state.ID.ValueString()), &tmpl)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage incident template",
			fmt.Sprintf("Could not read incident template %q on page %q: %s. Verify the template exists and has not been deleted.",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(tmpl.ID)
	state.PageID = types.StringValue(tmpl.PageID)
	state.Name = types.StringValue(tmpl.Name)
	state.Title = types.StringValue(tmpl.Title)
	state.Body = types.StringValue(tmpl.Body)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing incident template.
func (r *IncidentTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan IncidentTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state IncidentTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiIncidentTemplateCreate{
		Template: apiIncidentTemplateBody{
			Name: plan.Name.ValueString(),
		},
	}
	if !plan.Title.IsNull() && !plan.Title.IsUnknown() {
		body.Template.Title = plan.Title.ValueString()
	}
	if !plan.Body.IsNull() && !plan.Body.IsUnknown() {
		body.Template.Body = plan.Body.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiIncidentTemplate
	err := r.client.Put(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates/%s", state.PageID.ValueString(), state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Incident template not found",
					fmt.Sprintf("Incident template %q on page %q not found. The template may have been deleted outside of Terraform.",
						state.ID.ValueString(), state.PageID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update incident templates. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Statuspage incident template",
			fmt.Sprintf("Could not update incident template %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.PageID = types.StringValue(updated.PageID)
	plan.Name = types.StringValue(updated.Name)
	plan.Title = types.StringValue(updated.Title)
	plan.Body = types.StringValue(updated.Body)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an incident template.
func (r *IncidentTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state IncidentTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates/%s", state.PageID.ValueString(), state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete incident template %q. "+
						"Ensure the service account has Statuspage admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Statuspage incident template",
			fmt.Sprintf("Could not delete incident template %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing incident template by composite ID (page_id/template_id).
func (r *IncidentTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
