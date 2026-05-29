// Package organization implements the atlassian_organization managed resource.
//
// This resource adopts an existing Atlassian Cloud organization. Atlassian does
// not expose an API for creating organizations, so this resource reads an
// existing organization by ID and manages it in Terraform state.
package organization

import (
	"context"
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

// apiOrganizationResponse represents the JSON envelope from the Atlassian Admin API.
type apiOrganizationResponse struct {
	Data apiOrganization `json:"data"`
}

// apiOrganization represents an organization in the Atlassian Admin API.
type apiOrganization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// ResourceModel describes the organization resource data model.
type ResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Type types.String `tfsdk:"type"`
}

// Resource implements the atlassian_organization managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

// Schema defines the schema for the organization resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Adopts an existing Atlassian Cloud organization into Terraform state. " +
			"Atlassian does not provide an API for creating organizations, so this resource " +
			"reads an existing organization by ID and tracks it in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the Atlassian organization.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the organization.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of the organization.",
				Computed:    true,
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

// Create adopts an existing Atlassian organization by reading it from the API.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var apiResp apiOrganizationResponse
	err := r.client.AdminGet(ctx, fmt.Sprintf("/v1/orgs/%s", plan.ID.ValueString()), &apiResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Organization not found",
				fmt.Sprintf("Atlassian organization %q not found. Verify the organization ID is correct and you have admin access.",
					plan.ID.ValueString()),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read organization",
			fmt.Sprintf("Could not read Atlassian organization %q: %s", plan.ID.ValueString(), err.Error()),
		)
		return
	}

	org := apiResp.Data
	plan.ID = types.StringValue(org.ID)
	plan.Name = types.StringValue(org.Name)
	plan.Type = types.StringValue(org.Type)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var readResp apiOrganizationResponse
	err := r.client.AdminGet(ctx, fmt.Sprintf("/v1/orgs/%s", state.ID.ValueString()), &readResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read organization",
			fmt.Sprintf("Could not read Atlassian organization %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	org := readResp.Data
	state.ID = types.StringValue(org.ID)
	state.Name = types.StringValue(org.Name)
	state.Type = types.StringValue(org.Type)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is not supported for organizations.
func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Atlassian organizations cannot be updated through the API. "+
			"To change the organization, remove this resource from state and adopt a different organization.",
	)
}

// Delete removes the organization from Terraform state only.
// The organization continues to exist in Atlassian Cloud.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Organization not deleted from Atlassian",
		"The organization has been removed from Terraform state but continues to exist in Atlassian Cloud. "+
			"Atlassian does not provide an API for deleting organizations.",
	)
}

// ImportState imports an existing organization by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
