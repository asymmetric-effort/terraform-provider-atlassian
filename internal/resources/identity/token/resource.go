// Package token implements the atlassian_api_token managed resource.
//
// This resource manages Atlassian Cloud API tokens for service accounts
// through the Atlassian REST API. Tokens are immutable — any change to
// label or user_account_id forces replacement. The token_value is only
// available at creation time and is marked sensitive.
package token

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the Resource type satisfies framework interfaces.
var (
	_ resource.Resource                = &Resource{}
	_ resource.ResourceWithImportState = &Resource{}
)

// apiToken represents the JSON structure returned by the Atlassian token API.
type apiToken struct {
	TokenID   string `json:"tokenId"`
	Label     string `json:"label"`
	CreatedAt string `json:"createdAt"`
}

// apiTokenCreate represents the JSON body for creating a token.
type apiTokenCreate struct {
	Label string `json:"label"`
}

// apiTokenCreateResponse represents the JSON response after creating a token.
// It includes the token value which is only available at creation time.
type apiTokenCreateResponse struct {
	TokenID    string `json:"tokenId"`
	Label      string `json:"label"`
	TokenValue string `json:"tokenValue"`
	CreatedAt  string `json:"createdAt"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	TokenID       types.String `tfsdk:"token_id"`
	Label         types.String `tfsdk:"label"`
	UserAccountID types.String `tfsdk:"user_account_id"`
	TokenValue    types.String `tfsdk:"token_value"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

// Resource implements the atlassian_api_token managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_token"
}

// Schema defines the schema for the API token resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an Atlassian Cloud API token for a service account.",
		Attributes: map[string]schema.Attribute{
			"token_id": schema.StringAttribute{
				Description: "The unique identifier of the API token, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"label": schema.StringAttribute{
				Description: "A human-readable label for the API token.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"user_account_id": schema.StringAttribute{
				Description: "The account ID of the user who owns the API token.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"token_value": schema.StringAttribute{
				Description: "The API token value. Only available at creation time.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.StringAttribute{
				Description: "The timestamp when the API token was created.",
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

// Create generates a new API token for the specified user.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiTokenCreate{
		Label: plan.Label.ValueString(),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to marshal token create request", err.Error())
		return
	}

	endpoint := fmt.Sprintf("/rest/api/3/user/%s/token", plan.UserAccountID.ValueString())

	var created apiTokenCreateResponse
	err = r.client.Post(ctx, endpoint, bytes.NewReader(bodyBytes), &created)
	if err != nil {
		msg := err.Error()
		switch {
		case isStatusCode(err, http.StatusNotFound):
			msg = fmt.Sprintf("User with account ID %q not found. Verify the user_account_id is correct and the user exists.", plan.UserAccountID.ValueString())
		case isStatusCode(err, http.StatusForbidden):
			msg = "Permission denied: the authenticated user does not have permission to create API tokens for this user. Ensure the service account has the required privileges."
		case isStatusCode(err, http.StatusConflict):
			msg = "Token limit exceeded: the user has reached the maximum number of API tokens. Revoke an existing token before creating a new one."
		}
		resp.Diagnostics.AddError("Failed to create API token", msg)
		return
	}

	plan.TokenID = types.StringValue(created.TokenID)
	plan.TokenValue = types.StringValue(created.TokenValue)
	plan.CreatedAt = types.StringValue(created.CreatedAt)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read fetches the API token metadata (not the token value) from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := fmt.Sprintf("/rest/api/3/user/%s/token/%s",
		state.UserAccountID.ValueString(),
		state.TokenID.ValueString(),
	)

	var token apiToken
	err := r.client.Get(ctx, endpoint, &token)
	if err != nil {
		if isStatusCode(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read API token",
			fmt.Sprintf("Could not read API token %s for user %s: %s",
				state.TokenID.ValueString(),
				state.UserAccountID.ValueString(),
				err.Error(),
			),
		)
		return
	}

	state.TokenID = types.StringValue(token.TokenID)
	state.Label = types.StringValue(token.Label)
	state.CreatedAt = types.StringValue(token.CreatedAt)
	// token_value is not returned on read — preserve existing state value

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is not supported for API tokens — they are immutable.
// All mutable attributes use RequiresReplace, so this method should never be called.
func (r *Resource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"API tokens are immutable. Changes to label or user_account_id require replacing the token.",
	)
}

// Delete revokes an existing API token.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := fmt.Sprintf("/rest/api/3/user/%s/token/%s",
		state.UserAccountID.ValueString(),
		state.TokenID.ValueString(),
	)

	err := r.client.Delete(ctx, endpoint)
	if err != nil {
		if isStatusCode(err, http.StatusNotFound) {
			// Already revoked, nothing to do.
			return
		}
		msg := err.Error()
		if isStatusCode(err, http.StatusForbidden) {
			msg = "Permission denied: the authenticated user does not have permission to revoke API tokens for this user. Ensure the service account has the required privileges."
		}
		resp.Diagnostics.AddError("Failed to revoke API token", msg)
		return
	}
}

// ImportState imports an existing API token by token_id.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("token_id"), req, resp)
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
