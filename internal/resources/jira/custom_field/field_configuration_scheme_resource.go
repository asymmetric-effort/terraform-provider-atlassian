// Package customfield implements the atlassian_jira_field_configuration_scheme managed resource.
//
// This resource manages Jira field configuration schemes through the Atlassian Cloud REST API.
// It supports full CRUD operations and state import via field configuration scheme ID.
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

// Ensure the FieldConfigurationSchemeResource type satisfies framework interfaces.
var (
	_ resource.Resource                = &FieldConfigurationSchemeResource{}
	_ resource.ResourceWithImportState = &FieldConfigurationSchemeResource{}
)

// apiFieldConfigurationScheme represents the JSON structure returned by the Atlassian field configuration scheme API.
type apiFieldConfigurationScheme struct {
	ID          interface{} `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Self        string      `json:"self"`
}

// apiFieldConfigurationSchemeList represents the paginated list response from the field configuration scheme API.
type apiFieldConfigurationSchemeList struct {
	Values []apiFieldConfigurationScheme `json:"values"`
}

// apiFieldConfigurationSchemeCreate represents the JSON body for creating a field configuration scheme.
type apiFieldConfigurationSchemeCreate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// apiFieldConfigurationSchemeUpdate represents the JSON body for updating a field configuration scheme.
type apiFieldConfigurationSchemeUpdate struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// FieldConfigurationSchemeResourceModel describes the field configuration scheme resource data model.
type FieldConfigurationSchemeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
}

// FieldConfigurationSchemeResource implements the atlassian_jira_field_configuration_scheme managed resource.
type FieldConfigurationSchemeResource struct {
	client *atlassian.Client
}

// NewFieldConfigurationSchemeResource returns a new FieldConfigurationSchemeResource instance for provider registration.
func NewFieldConfigurationSchemeResource() resource.Resource {
	return &FieldConfigurationSchemeResource{}
}

// Metadata returns the resource type name.
func (r *FieldConfigurationSchemeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_field_configuration_scheme"
}

// Schema defines the schema for the jira field configuration scheme resource.
func (r *FieldConfigurationSchemeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira field configuration scheme in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the field configuration scheme, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the field configuration scheme.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the field configuration scheme.",
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
func (r *FieldConfigurationSchemeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create provisions a new Jira field configuration scheme.
func (r *FieldConfigurationSchemeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan FieldConfigurationSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiFieldConfigurationSchemeCreate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiFieldConfigurationScheme
	err := r.client.Post(ctx, "/rest/api/3/fieldconfigurationscheme", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate field configuration scheme name",
					fmt.Sprintf("A field configuration scheme with name %q already exists. Each field configuration scheme name must be unique.", plan.Name.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid field configuration scheme",
					fmt.Sprintf("The field configuration scheme %q is invalid. Verify the scheme name and description are correct.",
						plan.Name.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create field configuration schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create field configuration scheme",
			fmt.Sprintf("Could not create field configuration scheme with name %q: %s", plan.Name.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(idToString(created.ID))
	plan.Name = types.StringValue(created.Name)
	plan.Description = types.StringValue(created.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *FieldConfigurationSchemeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state FieldConfigurationSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var list apiFieldConfigurationSchemeList
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/fieldconfigurationscheme?id=%s", state.ID.ValueString()), &list)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read field configuration scheme",
			fmt.Sprintf("Could not read field configuration scheme %q: %s. Verify the field configuration scheme exists and has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	if len(list.Values) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	fcs := list.Values[0]
	state.ID = types.StringValue(idToString(fcs.ID))
	state.Name = types.StringValue(fcs.Name)
	state.Description = types.StringValue(fcs.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Jira field configuration scheme.
func (r *FieldConfigurationSchemeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan FieldConfigurationSchemeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state FieldConfigurationSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiFieldConfigurationSchemeUpdate{
		Name: plan.Name.ValueString(),
	}
	if !plan.Description.IsNull() && !plan.Description.IsUnknown() {
		body.Description = plan.Description.ValueString()
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiFieldConfigurationScheme
	err := r.client.Put(ctx, fmt.Sprintf("/rest/api/3/fieldconfigurationscheme/%s", state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Field configuration scheme not found",
					fmt.Sprintf("Field configuration scheme with ID %q not found. The field configuration scheme may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid field configuration scheme",
					fmt.Sprintf("The field configuration scheme update for ID %q is invalid. Verify the scheme name and description are correct.",
						state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update field configuration schemes. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update field configuration scheme",
			fmt.Sprintf("Could not update field configuration scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(idToString(updated.ID))
	plan.Name = types.StringValue(updated.Name)
	plan.Description = types.StringValue(updated.Description)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Jira field configuration scheme.
func (r *FieldConfigurationSchemeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state FieldConfigurationSchemeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/fieldconfigurationscheme/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete field configuration scheme %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete field configuration scheme",
			fmt.Sprintf("Could not delete field configuration scheme with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing field configuration scheme by ID.
func (r *FieldConfigurationSchemeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
