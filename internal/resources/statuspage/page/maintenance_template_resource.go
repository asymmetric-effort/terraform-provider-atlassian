// Package page implements the atlassian_statuspage_maintenance_template managed resource.
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

// Ensure the MaintenanceTemplateResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &MaintenanceTemplateResource{}
	_ resource.ResourceWithImportState = &MaintenanceTemplateResource{}
)

// apiMaintenanceTemplate represents the JSON structure returned by the maintenance template API.
type apiMaintenanceTemplate struct {
	ID     string `json:"id"`
	PageID string `json:"page_id"`
	Name   string `json:"name"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// apiMaintenanceTemplateCreate represents the JSON body for creating a maintenance template.
type apiMaintenanceTemplateCreate struct {
	Template apiMaintenanceTemplateBody `json:"template"`
}

// apiMaintenanceTemplateBody holds the template fields.
type apiMaintenanceTemplateBody struct {
	Name  string `json:"name"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

// MaintenanceTemplateResourceModel describes the resource data model.
type MaintenanceTemplateResourceModel struct {
	ID     types.String `tfsdk:"id"`
	PageID types.String `tfsdk:"page_id"`
	Name   types.String `tfsdk:"name"`
	Title  types.String `tfsdk:"title"`
	Body   types.String `tfsdk:"body"`
}

// MaintenanceTemplateResource implements the atlassian_statuspage_maintenance_template managed resource.
type MaintenanceTemplateResource struct {
	client *atlassian.Client
}

// NewMaintenanceTemplateResource returns a new MaintenanceTemplateResource instance for provider registration.
func NewMaintenanceTemplateResource() resource.Resource {
	return &MaintenanceTemplateResource{}
}

// Metadata returns the resource type name.
func (r *MaintenanceTemplateResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_statuspage_maintenance_template"
}

// Schema defines the schema for the maintenance template resource.
func (r *MaintenanceTemplateResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Statuspage maintenance template.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the maintenance template.",
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
				Description: "The name of the maintenance template.",
				Required:    true,
			},
			"title": schema.StringAttribute{
				Description: "The default title for maintenance events created from this template.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"body": schema.StringAttribute{
				Description: "The default body for maintenance events created from this template.",
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
func (r *MaintenanceTemplateResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new maintenance template.
func (r *MaintenanceTemplateResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MaintenanceTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiMaintenanceTemplateCreate{
		Template: apiMaintenanceTemplateBody{
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

	var created apiMaintenanceTemplate
	err := r.client.Post(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates", plan.PageID.ValueString()), bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusForbidden {
			resp.Diagnostics.AddError(
				"Permission denied",
				"The authenticated user does not have permission to create maintenance templates. Ensure the service account has Statuspage admin privileges.",
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to create Statuspage maintenance template",
			fmt.Sprintf("Could not create maintenance template with name %q on page %q: %s",
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
func (r *MaintenanceTemplateResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MaintenanceTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var tmpl apiMaintenanceTemplate
	err := r.client.Get(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", state.PageID.ValueString(), state.ID.ValueString()), &tmpl)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read Statuspage maintenance template",
			fmt.Sprintf("Could not read maintenance template %q on page %q: %s. Verify the template exists and has not been deleted.",
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

// Update modifies an existing maintenance template.
func (r *MaintenanceTemplateResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MaintenanceTemplateResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state MaintenanceTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiMaintenanceTemplateCreate{
		Template: apiMaintenanceTemplateBody{
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

	var updated apiMaintenanceTemplate
	err := r.client.Put(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", state.PageID.ValueString(), state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Maintenance template not found",
					fmt.Sprintf("Maintenance template %q on page %q not found. The template may have been deleted outside of Terraform.",
						state.ID.ValueString(), state.PageID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update maintenance templates. Ensure the service account has Statuspage admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update Statuspage maintenance template",
			fmt.Sprintf("Could not update maintenance template %q on page %q: %s",
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

// Delete removes a maintenance template.
func (r *MaintenanceTemplateResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MaintenanceTemplateResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", state.PageID.ValueString(), state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete maintenance template %q. "+
						"Ensure the service account has Statuspage admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete Statuspage maintenance template",
			fmt.Sprintf("Could not delete maintenance template %q on page %q: %s",
				state.ID.ValueString(), state.PageID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing maintenance template by composite ID (page_id/template_id).
func (r *MaintenanceTemplateResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
