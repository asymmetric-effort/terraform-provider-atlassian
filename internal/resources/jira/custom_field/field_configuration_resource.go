// Package customfield implements the atlassian_jira_field_configuration managed resource.
//
// This resource manages Jira field configurations through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via field configuration ID.
package customfield

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

// Ensure the FieldConfigurationResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &FieldConfigurationResource{}
	_ resource.ResourceWithImportState = &FieldConfigurationResource{}
)

// apiFieldConfiguration represents the JSON structure returned by the Atlassian field configuration API.
type apiFieldConfiguration struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Self        string `json:"self"`
}

// apiFieldConfigurationCreate represents the JSON body for creating a field configuration.
type apiFieldConfigurationCreate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// apiFieldConfigurationUpdate represents the JSON body for updating a field configuration.
type apiFieldConfigurationUpdate struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// FieldConfigurationResourceModel describes the field configuration resource data model.
type FieldConfigurationResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// FieldConfigurationResource implements the atlassian_jira_field_configuration managed resource.
type FieldConfigurationResource struct {
	client *atlassian.Client
}

// NewFieldConfigurationResource returns a new FieldConfigurationResource instance for provider registration.
func NewFieldConfigurationResource() resource.Resource {
	return &FieldConfigurationResource{}
}

// Metadata returns the resource type name.
func (r *FieldConfigurationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_field_configuration"
}

// Schema defines the schema for the jira field configuration resource.
func (r *FieldConfigurationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira field configuration in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the field configuration, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the field configuration.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the field configuration.",
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
func (r *FieldConfigurationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira field configuration.
func (r *FieldConfigurationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FieldConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiFieldConfigurationCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiFieldConfiguration
	err := r.client.Post(ctx, "/rest/api/3/fieldconfiguration", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate field configuration name",
					fmt.Sprintf("A field configuration with name %q already exists. Each field configuration name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid field configuration",
					fmt.Sprintf("The field configuration %q is invalid. Verify the configuration name and description are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create field configurations. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create field configuration",
			fmt.Sprintf("Could not create field configuration with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.ID)
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *FieldConfigurationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FieldConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var fc apiFieldConfiguration
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/fieldconfiguration/%s", state.ID.ValueString()), &fc)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read field configuration",
			fmt.Sprintf("Could not read field configuration %q: %s. Verify the field configuration exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(fc.ID)
	state.Name = types.StringValue(fc.Name)
	state.Description = types.StringValue(fc.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira field configuration.
func (r *FieldConfigurationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FieldConfigurationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state FieldConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiFieldConfigurationUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiFieldConfiguration
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/fieldconfiguration/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Field configuration not found",
					fmt.Sprintf("Field configuration with ID %q not found. The field configuration may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid field configuration",
					fmt.Sprintf("The field configuration update for ID %q is invalid. Verify the configuration name and description are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update field configurations. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update field configuration",
			fmt.Sprintf("Could not update field configuration with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.ID)
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira field configuration.
func (r *FieldConfigurationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FieldConfigurationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/fieldconfiguration/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete field configuration %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete field configuration",
			fmt.Sprintf("Could not delete field configuration with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing field configuration by ID.
func (r *FieldConfigurationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
