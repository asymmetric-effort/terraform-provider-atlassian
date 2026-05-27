// Package user implements the atlassian_user managed resource.
//
// This resource manages Atlassian Cloud user accounts through the
// Atlassian Admin API. It supports full CRUD operations and state
// import via account ID.
package user

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiUser represents the JSON structure returned by the Atlassian user API.
type apiUser struct {
	AccountID   string `json:"accountId"`
	Email       string `json:"emailAddress"`
	DisplayName string `json:"displayName"`
	Active      bool   `json:"active"`
}

// apiUserCreate represents the JSON body for creating a user.
type apiUserCreate struct {
	Email       string `json:"emailAddress"`
	DisplayName string `json:"displayName"`
}

// apiUserUpdate represents the JSON body for updating a user.
type apiUserUpdate struct {
	DisplayName string `json:"displayName,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	AccountID   types.String `tfsdk:"account_id"`
	Email       types.String `tfsdk:"email"`
	DisplayName types.String `tfsdk:"display_name"`
	Active      types.Bool   `tfsdk:"active"`
}

// Resource implements the atlassian_user managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// Schema defines the schema for the user resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Atlassian Cloud user account.",
		Attributes: map[string]schema.Attribute{
			"account_id": schema.StringAttribute{
				Description: "The unique account ID of the user, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"email": schema.StringAttribute{
				Description: "The email address of the user. Must be unique across the Atlassian organization.",
				Required:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "The display name of the user.",
				Required:    true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the user account is active.",
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
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

// Create provisions a new Atlassian user account.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiUserCreate{
		Email:       plan.Email.ValueString(),
		DisplayName: plan.DisplayName.ValueString(),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to marshal user create request", err.Error())
		return
	}

	var created apiUser
	err = r.client.Post(ctx, "/rest/api/3/user", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		msg := err.Error()
		switch {
		case isStatusCode(err, http.StatusConflict):
			msg = fmt.Sprintf("A user with email %q already exists. Each email must be unique within the Atlassian organization.", plan.Email.ValueString())
		case isStatusCode(err, http.StatusForbidden):
			msg = "Permission denied: the authenticated user does not have permission to create users. Ensure the service account has organization admin privileges."
		}
		resp.Diagnostics.AddError("Failed to create user", msg)
		return
	}

	plan.AccountID = types.StringValue(created.AccountID)
	plan.Email = types.StringValue(created.Email)
	plan.DisplayName = types.StringValue(created.DisplayName)
	plan.Active = types.BoolValue(created.Active)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var user apiUser
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", state.AccountID.ValueString()), &user)
	if err != nil {
		if isStatusCode(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read user",
			fmt.Sprintf("Could not read user with account ID %s: %s", state.AccountID.ValueString(), err.Error()),
		)
		return
	}

	state.AccountID = types.StringValue(user.AccountID)
	state.Email = types.StringValue(user.Email)
	state.DisplayName = types.StringValue(user.DisplayName)
	state.Active = types.BoolValue(user.Active)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Atlassian user account.
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

	body := apiUserUpdate{
		DisplayName: plan.DisplayName.ValueString(),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to marshal user update request", err.Error())
		return
	}

	var updated apiUser
	err = r.client.Put(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", state.AccountID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		msg := err.Error()
		switch {
		case isStatusCode(err, http.StatusNotFound):
			msg = fmt.Sprintf("User with account ID %s not found. The user may have been deleted outside of Terraform.", state.AccountID.ValueString())
		case isStatusCode(err, http.StatusForbidden):
			msg = "Permission denied: the authenticated user does not have permission to update users. Ensure the service account has organization admin privileges."
		}
		resp.Diagnostics.AddError("Failed to update user", msg)
		return
	}

	plan.AccountID = types.StringValue(updated.AccountID)
	plan.Email = types.StringValue(updated.Email)
	plan.DisplayName = types.StringValue(updated.DisplayName)
	plan.Active = types.BoolValue(updated.Active)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes an Atlassian user account.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", state.AccountID.ValueString()))
	if err != nil {
		if isStatusCode(err, http.StatusNotFound) {
			// Already deleted, nothing to do.
			return
		}
		msg := err.Error()
		if isStatusCode(err, http.StatusForbidden) {
			msg = "Permission denied: the authenticated user does not have permission to delete users. Ensure the service account has organization admin privileges."
		}
		resp.Diagnostics.AddError("Failed to delete user", msg)
		return
	}
}

// ImportState imports an existing user by account ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("account_id"), req, resp)
}

// isStatusCode checks whether an error is an APIError with the given HTTP status code.
func isStatusCode(err error, code int) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	expected := fmt.Sprintf("HTTP %d)", code)
	return strings.Contains(msg, expected)
}
