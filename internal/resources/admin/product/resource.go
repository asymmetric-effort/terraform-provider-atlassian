// Package product implements the atlassian_product managed resource.
//
// This resource provisions an Atlassian Cloud product instance (e.g., Jira Software,
// Confluence) within an organization using the Admin Installations API.
package product

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// provisioningTimeout is the maximum time to wait for product provisioning.
var provisioningTimeout = 10 * time.Minute

// provisioningPollInterval is the initial interval between status checks.
var provisioningPollInterval = 5 * time.Second

// apiProvisionRequest represents the JSON body for provisioning a product.
type apiProvisionRequest struct {
	Offerings  []apiOffering      `json:"offerings"`
	Parameters apiProvisionParams `json:"parameters"`
}

// apiOffering represents a product offering in a provision request.
type apiOffering struct {
	ID       string `json:"id"`
	Location string `json:"location"`
}

// apiProvisionParams represents the parameters for provisioning.
type apiProvisionParams struct {
	AdminEmail string `json:"adminEmail"`
	Name       string `json:"name"`
	Timezone   string `json:"timezone,omitempty"`
}

// apiProvisionResponse represents the response from the provisioning API.
type apiProvisionResponse struct {
	RequestID string `json:"requestId"`
	StatusURL string `json:"statusUrl"`
}

// apiProvisionStatus represents the provisioning status response.
type apiProvisionStatus struct {
	Data struct {
		RequestID string `json:"requestId"`
		Status    string `json:"status"`
	} `json:"data"`
}

// apiWorkspaceQuery represents the workspace query request.
type apiWorkspaceQuery struct {
	Query struct {
		Field struct {
			Name   string   `json:"name"`
			Values []string `json:"values"`
		} `json:"field"`
	} `json:"query"`
}

// apiWorkspaceResponse represents the workspace query response.
type apiWorkspaceResponse struct {
	Data []struct {
		ID         string `json:"id"`
		Attributes struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"attributes"`
	} `json:"data"`
}

// ResourceModel describes the product resource data model.
type ResourceModel struct {
	ID         types.String `tfsdk:"id"`
	OrgID      types.String `tfsdk:"org_id"`
	OfferingID types.String `tfsdk:"offering_id"`
	SiteName   types.String `tfsdk:"site_name"`
	Location   types.String `tfsdk:"location"`
	AdminEmail types.String `tfsdk:"admin_email"`
	Timezone   types.String `tfsdk:"timezone"`
	SiteURL    types.String `tfsdk:"site_url"`
	Status     types.String `tfsdk:"status"`
	RequestID  types.String `tfsdk:"request_id"`
}

// Resource implements the atlassian_product managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_product"
}

// Schema defines the schema for the product resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provisions an Atlassian Cloud product instance (e.g., Jira Software, Confluence) " +
			"within an organization. Uses the Admin Installations API to create a new site with " +
			"the specified product.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the provisioned workspace.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"org_id": schema.StringAttribute{
				Description: "The ID of the parent Atlassian organization.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"offering_id": schema.StringAttribute{
				Description: "The product offering ID (UUID) to provision.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"site_name": schema.StringAttribute{
				Description: "The desired site name (e.g., 'my-company' creates my-company.atlassian.net).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"location": schema.StringAttribute{
				Description: "The region for the product instance (e.g., 'us', 'eu').",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"admin_email": schema.StringAttribute{
				Description: "The email address of the site administrator.",
				Required:    true,
			},
			"timezone": schema.StringAttribute{
				Description: "The timezone for the product instance. Defaults to UTC.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("UTC"),
			},
			"site_url": schema.StringAttribute{
				Description: "The URL of the provisioned site (e.g., https://my-company.atlassian.net).",
				Computed:    true,
			},
			"status": schema.StringAttribute{
				Description: "The provisioning status (e.g., COMPLETED, IN_PROGRESS, FAILED).",
				Computed:    true,
			},
			"request_id": schema.StringAttribute{
				Description: "The provisioning request ID for tracking.",
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

// Create provisions a new Atlassian product instance.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	orgID := plan.OrgID.ValueString()

	// Build provisioning request
	provReq := apiProvisionRequest{
		Offerings: []apiOffering{
			{
				ID:       plan.OfferingID.ValueString(),
				Location: plan.Location.ValueString(),
			},
		},
		Parameters: apiProvisionParams{
			AdminEmail: plan.AdminEmail.ValueString(),
			Name:       plan.SiteName.ValueString(),
			Timezone:   plan.Timezone.ValueString(),
		},
	}
	bodyBytes, _ := json.Marshal(provReq)

	// Submit provisioning request
	var provResp apiProvisionResponse
	err := r.client.AdminPost(ctx,
		fmt.Sprintf("/installations/v2/orgs/%s/products", orgID),
		bytes.NewReader(bodyBytes), &provResp)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid provisioning request",
					fmt.Sprintf("The product provisioning request is invalid: %s", apiErr.Message),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to provision products. Ensure the account has organization admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to provision product",
			fmt.Sprintf("Could not provision product in organization %q: %s", orgID, err.Error()),
		)
		return
	}

	// Poll for provisioning completion (iterative, no recursion)
	deadline := time.Now().Add(provisioningTimeout)
	pollInterval := provisioningPollInterval
	var finalStatus string

	for time.Now().Before(deadline) {
		var status apiProvisionStatus
		statusErr := r.client.AdminGet(ctx,
			fmt.Sprintf("/installations/v2/orgs/%s/products/status/%s", orgID, provResp.RequestID),
			&status)
		if statusErr != nil {
			resp.Diagnostics.AddError(
				"Failed to check provisioning status",
				fmt.Sprintf("Could not check provisioning status: %s", statusErr.Error()),
			)
			return
		}

		finalStatus = status.Data.Status
		if finalStatus == "COMPLETED" || finalStatus == "FAILED" {
			break
		}

		select {
		case <-ctx.Done():
			resp.Diagnostics.AddError("Provisioning cancelled", "The provisioning operation was cancelled.")
			return
		case <-time.After(pollInterval):
		}

		// Increase poll interval (capped at 30s)
		pollInterval = pollInterval * 2
		if pollInterval > 30*time.Second {
			pollInterval = 30 * time.Second
		}
	}

	if finalStatus == "FAILED" {
		resp.Diagnostics.AddError(
			"Product provisioning failed",
			fmt.Sprintf("Provisioning request %q failed. Check the Atlassian admin console for details.", provResp.RequestID),
		)
		return
	}
	if finalStatus != "COMPLETED" {
		resp.Diagnostics.AddError(
			"Product provisioning timed out",
			fmt.Sprintf("Provisioning request %q did not complete within the timeout period. Current status: %s", provResp.RequestID, finalStatus),
		)
		return
	}

	// Query workspaces to find the newly created site
	siteURL, wsID := r.findWorkspace(ctx, orgID, plan.SiteName.ValueString())

	plan.ID = types.StringValue(wsID)
	plan.SiteURL = types.StringValue(siteURL)
	plan.Status = types.StringValue(finalStatus)
	plan.RequestID = types.StringValue(provResp.RequestID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest workspace data.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteURL, wsID := r.findWorkspace(ctx, state.OrgID.ValueString(), state.SiteName.ValueString())
	if wsID == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	state.ID = types.StringValue(wsID)
	state.SiteURL = types.StringValue(siteURL)
	state.Status = types.StringValue("COMPLETED")

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is limited — most product attributes are immutable after provisioning.
func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Only admin_email and timezone can potentially be updated.
	// For now, just refresh state from API.
	siteURL, wsID := r.findWorkspace(ctx, plan.OrgID.ValueString(), plan.SiteName.ValueString())
	if wsID == "" {
		resp.Diagnostics.AddError(
			"Workspace not found",
			fmt.Sprintf("Could not find workspace %q in organization %q.", plan.SiteName.ValueString(), plan.OrgID.ValueString()),
		)
		return
	}

	plan.ID = types.StringValue(wsID)
	plan.SiteURL = types.StringValue(siteURL)
	plan.Status = types.StringValue("COMPLETED")

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes the product from Terraform state.
// Product deprovisioning is not supported via API.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Product not deprovisioned from Atlassian",
		"The product has been removed from Terraform state but continues to exist in Atlassian Cloud. "+
			"Product deprovisioning must be performed through the Atlassian admin console.",
	)
}

// ImportState imports an existing product by org_id/site_name.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// findWorkspace queries workspaces to find one matching the given site name.
func (r *Resource) findWorkspace(ctx context.Context, orgID, siteName string) (siteURL, wsID string) {
	query := apiWorkspaceQuery{}
	query.Query.Field.Name = "attributes.name"
	query.Query.Field.Values = []string{siteName}
	bodyBytes, _ := json.Marshal(query)

	var wsResp apiWorkspaceResponse
	err := r.client.AdminPost(ctx,
		fmt.Sprintf("/v2/orgs/%s/workspaces", orgID),
		bytes.NewReader(bodyBytes), &wsResp)
	if err != nil {
		return "", ""
	}

	for _, ws := range wsResp.Data {
		if ws.Attributes.Name == siteName {
			return ws.Attributes.URL, ws.ID
		}
	}
	return "", ""
}
