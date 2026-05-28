package group

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure MembershipResource satisfies required interfaces.
var (
	_ resource.Resource                = &MembershipResource{}
	_ resource.ResourceWithImportState = &MembershipResource{}
)

// MembershipResource implements the atlassian_group_membership managed resource.
type MembershipResource struct {
	client *atlassian.Client
}

// MembershipResourceModel describes the resource data model for group membership.
type MembershipResourceModel struct {
	GroupID        types.String `tfsdk:"group_id"`
	UserAccountIDs types.List   `tfsdk:"user_account_ids"`
}

// apiGroupMemberResponse represents a single member in the group member list API response.
type apiGroupMemberResponse struct {
	AccountID string `json:"accountId"`
}

// apiGroupMemberListResponse represents the JSON response from the group member list API.
type apiGroupMemberListResponse struct {
	Values     []apiGroupMemberResponse `json:"values"`
	MaxResults int                      `json:"maxResults"`
	StartAt    int                      `json:"startAt"`
	Total      int                      `json:"total"`
	IsLast     bool                     `json:"isLast"`
}

// apiAddUserToGroupRequest represents the JSON body for adding a user to a group.
type apiAddUserToGroupRequest struct {
	AccountID string `json:"accountId"`
}

// NewMembershipResource returns a new instance of the group membership resource.
func NewMembershipResource() resource.Resource {
	return &MembershipResource{}
}

// Metadata returns the resource type name.
func (r *MembershipResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group_membership"
}

// Schema defines the schema for the group membership resource.
func (r *MembershipResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages user-to-group membership assignments in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"group_id": schema.StringAttribute{
				Description: "The unique identifier of the group to manage membership for.",
				Required:    true,
			},
			"user_account_ids": schema.ListAttribute{
				Description: "The list of user account IDs that should be members of the group.",
				Required:    true,
				ElementType: types.StringType,
			},
		},
	}
}

// Configure retrieves the provider-configured client for API calls.
func (r *MembershipResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create adds all specified users to the group.
func (r *MembershipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan MembershipResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := plan.GroupID.ValueString()

	var userIDs []string
	resp.Diagnostics.Append(plan.UserAccountIDs.ElementsAs(ctx, &userIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, accountID := range userIDs {
		err := r.addUserToGroup(ctx, groupID, accountID)
		if err != nil {
			if apiErr, ok := err.(*atlassian.APIError); ok {
				switch apiErr.StatusCode {
				case 404:
					resp.Diagnostics.AddError(
						"Group or user not found",
						fmt.Sprintf("Could not add user %q to group %q: the group or user was not found. "+
							"Verify that both the group ID and user account ID are correct.", accountID, groupID),
					)
					return
				case 409:
					// User is already a member; this is acceptable for idempotent create.
					continue
				case 403:
					resp.Diagnostics.AddError(
						"Permission denied",
						fmt.Sprintf("The authenticated user does not have permission to add user %q to group %q. "+
							"Ensure the user has the 'Browse users and groups' global permission.", accountID, groupID),
					)
					return
				}
			}
			resp.Diagnostics.AddError(
				"Failed to add user to group",
				fmt.Sprintf("Could not add user %q to group %q: %s", accountID, groupID, err.Error()),
			)
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read retrieves the current group membership from the API.
func (r *MembershipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state MembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := state.GroupID.ValueString()

	members, err := r.listGroupMembers(ctx, groupID)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 404 {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read group membership",
			fmt.Sprintf("Could not read members of group %q: %s", groupID, err.Error()),
		)
		return
	}

	// Only track the users that are in both the API response and our managed state.
	var managedIDs []string
	// ElementsAs cannot fail here because the schema guarantees user_account_ids
	// is a list of strings and State.Get already validated the type above.
	state.UserAccountIDs.ElementsAs(ctx, &managedIDs, false)

	remoteSet := make(map[string]bool, len(members))
	for _, m := range members {
		remoteSet[m] = true
	}

	// Filter to only users we manage that still exist in the group.
	var currentIDs []string
	for _, id := range managedIDs {
		if remoteSet[id] {
			currentIDs = append(currentIDs, id)
		}
	}

	// ListValueFrom cannot fail with a []string and types.StringType.
	listVal, _ := types.ListValueFrom(ctx, types.StringType, currentIDs)

	state.UserAccountIDs = listVal
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update computes the diff between old and new membership and applies changes.
func (r *MembershipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan MembershipResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state MembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := plan.GroupID.ValueString()

	var planIDs []string
	resp.Diagnostics.Append(plan.UserAccountIDs.ElementsAs(ctx, &planIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var stateIDs []string
	resp.Diagnostics.Append(state.UserAccountIDs.ElementsAs(ctx, &stateIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldSet := make(map[string]bool, len(stateIDs))
	for _, id := range stateIDs {
		oldSet[id] = true
	}

	newSet := make(map[string]bool, len(planIDs))
	for _, id := range planIDs {
		newSet[id] = true
	}

	// Add new members.
	for _, id := range planIDs {
		if !oldSet[id] {
			err := r.addUserToGroup(ctx, groupID, id)
			if err != nil {
				if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 409 {
					// Already a member; continue.
					continue
				}
				resp.Diagnostics.AddError(
					"Failed to add user to group",
					fmt.Sprintf("Could not add user %q to group %q: %s", id, groupID, err.Error()),
				)
				return
			}
		}
	}

	// Remove old members.
	for _, id := range stateIDs {
		if !newSet[id] {
			err := r.removeUserFromGroup(ctx, groupID, id)
			if err != nil {
				if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == 404 {
					// User already removed; continue.
					continue
				}
				resp.Diagnostics.AddError(
					"Failed to remove user from group",
					fmt.Sprintf("Could not remove user %q from group %q: %s", id, groupID, err.Error()),
				)
				return
			}
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes all managed users from the group.
func (r *MembershipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state MembershipResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	groupID := state.GroupID.ValueString()

	var userIDs []string
	resp.Diagnostics.Append(state.UserAccountIDs.ElementsAs(ctx, &userIDs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, accountID := range userIDs {
		err := r.removeUserFromGroup(ctx, groupID, accountID)
		if err != nil {
			if apiErr, ok := err.(*atlassian.APIError); ok {
				switch apiErr.StatusCode {
				case 404:
					// Group or user already gone; nothing to do.
					continue
				}
			}
			resp.Diagnostics.AddError(
				"Failed to remove user from group",
				fmt.Sprintf("Could not remove user %q from group %q: %s", accountID, groupID, err.Error()),
			)
			return
		}
	}
}

// ImportState imports an existing group membership by group ID.
func (r *MembershipResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("group_id"), req, resp)
}

// addUserToGroup adds a single user to the group via the Atlassian REST API.
func (r *MembershipResource) addUserToGroup(ctx context.Context, groupID, accountID string) error {
	reqBody := apiAddUserToGroupRequest{
		AccountID: accountID,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	endpoint := fmt.Sprintf("/rest/api/3/group/user?groupId=%s", url.QueryEscape(groupID))
	return r.client.Post(ctx, endpoint, bytes.NewReader(bodyBytes), nil)
}

// removeUserFromGroup removes a single user from the group via the Atlassian REST API.
func (r *MembershipResource) removeUserFromGroup(ctx context.Context, groupID, accountID string) error {
	endpoint := fmt.Sprintf("/rest/api/3/group/user?groupId=%s&accountId=%s",
		url.QueryEscape(groupID), url.QueryEscape(accountID))
	return r.client.Delete(ctx, endpoint)
}

// listGroupMembers retrieves all member account IDs for a group, handling pagination.
func (r *MembershipResource) listGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	var allMembers []string
	startAt := 0

	for {
		endpoint := fmt.Sprintf("/rest/api/3/group/member?groupId=%s&startAt=%d&maxResults=50",
			url.QueryEscape(groupID), startAt)

		var apiResp apiGroupMemberListResponse
		err := r.client.Get(ctx, endpoint, &apiResp)
		if err != nil {
			return nil, err
		}

		for _, member := range apiResp.Values {
			allMembers = append(allMembers, member.AccountID)
		}

		if apiResp.IsLast || len(apiResp.Values) == 0 {
			break
		}

		startAt += len(apiResp.Values)
	}

	return allMembers, nil
}
