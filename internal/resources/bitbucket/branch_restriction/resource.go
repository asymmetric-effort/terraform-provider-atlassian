// Package branch_restriction implements the atlassian_bitbucket_branch_restriction managed resource.
//
// This resource manages Bitbucket branch restrictions through the Atlassian
// Cloud REST API. It supports full CRUD operations and state import
// via restriction ID.
package branch_restriction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
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

// apiBranchRestriction represents the JSON structure returned by the Bitbucket branch restrictions API.
type apiBranchRestriction struct {
	ID      int      `json:"id"`
	Kind    string   `json:"kind"`
	Pattern string   `json:"pattern"`
	Users   []string `json:"users"`
	Groups  []string `json:"groups"`
}

// apiBranchRestrictionCreate represents the JSON body for creating a branch restriction.
type apiBranchRestrictionCreate struct {
	Kind    string   `json:"kind"`
	Pattern string   `json:"pattern"`
	Users   []string `json:"users,omitempty"`
	Groups  []string `json:"groups,omitempty"`
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Repository types.String `tfsdk:"repository"`
	Pattern    types.String `tfsdk:"pattern"`
	Kind       types.String `tfsdk:"kind"`
	Users      types.List   `tfsdk:"users"`
	Groups     types.List   `tfsdk:"groups"`
}

// Resource implements the atlassian_bitbucket_branch_restriction managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_branch_restriction"
}

// Schema defines the schema for the bitbucket branch restriction resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Bitbucket branch restriction in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the branch restriction, assigned by Bitbucket.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"repository": schema.StringAttribute{
				Description: "The repository in workspace/slug format (e.g., myworkspace/myrepo).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"pattern": schema.StringAttribute{
				Description: "The branch pattern to restrict (e.g., main, release/*).",
				Required:    true,
			},
			"kind": schema.StringAttribute{
				Description: "The kind of restriction. Must be one of: push, delete, force, merge.",
				Required:    true,
			},
			"users": schema.ListAttribute{
				Description: "List of user account IDs to restrict.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
			},
			"groups": schema.ListAttribute{
				Description: "List of group slugs to restrict.",
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
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

// repoPath builds the API path prefix for a repository.
func repoPath(repository string) string {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("/2.0/repositories/%s/%s", parts[0], parts[1])
}

// Create provisions a new Bitbucket branch restriction.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := repoPath(plan.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError(
			"Invalid repository format",
			"Repository must be in workspace/slug format (e.g., myworkspace/myrepo).",
		)
		return
	}

	body := apiBranchRestrictionCreate{
		Kind:    plan.Kind.ValueString(),
		Pattern: plan.Pattern.ValueString(),
		Users:   listToStrings(ctx, plan.Users),
		Groups:  listToStrings(ctx, plan.Groups),
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiBranchRestriction
	err := r.client.Post(ctx, base+"/branch-restrictions", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create branch restrictions.",
				)
				return
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Repository not found",
					fmt.Sprintf("Repository %q not found. Verify the workspace/slug is correct.", plan.Repository.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create branch restriction",
			fmt.Sprintf("Could not create branch restriction: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", created.ID))
	plan.Pattern = types.StringValue(created.Pattern)
	plan.Kind = types.StringValue(created.Kind)
	plan.Users = stringsToList(ctx, created.Users)
	plan.Groups = stringsToList(ctx, created.Groups)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Bitbucket.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := repoPath(state.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError("Invalid repository format", "Repository must be in workspace/slug format.")
		return
	}

	var restriction apiBranchRestriction
	err := r.client.Get(ctx, fmt.Sprintf("%s/branch-restrictions/%s", base, state.ID.ValueString()), &restriction)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read branch restriction",
			fmt.Sprintf("Could not read branch restriction %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(fmt.Sprintf("%d", restriction.ID))
	state.Pattern = types.StringValue(restriction.Pattern)
	state.Kind = types.StringValue(restriction.Kind)
	state.Users = stringsToList(ctx, restriction.Users)
	state.Groups = stringsToList(ctx, restriction.Groups)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Bitbucket branch restriction.
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

	base := repoPath(state.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError("Invalid repository format", "Repository must be in workspace/slug format.")
		return
	}

	body := apiBranchRestrictionCreate{
		Kind:    plan.Kind.ValueString(),
		Pattern: plan.Pattern.ValueString(),
		Users:   listToStrings(ctx, plan.Users),
		Groups:  listToStrings(ctx, plan.Groups),
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiBranchRestriction
	err := r.client.Put(ctx, fmt.Sprintf("%s/branch-restrictions/%s", base, state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Branch restriction not found",
					fmt.Sprintf("Branch restriction %q not found. It may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update branch restrictions.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update branch restriction",
			fmt.Sprintf("Could not update branch restriction %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%d", updated.ID))
	plan.Pattern = types.StringValue(updated.Pattern)
	plan.Kind = types.StringValue(updated.Kind)
	plan.Users = stringsToList(ctx, updated.Users)
	plan.Groups = stringsToList(ctx, updated.Groups)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Bitbucket branch restriction.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	base := repoPath(state.Repository.ValueString())
	if base == "" {
		resp.Diagnostics.AddError("Invalid repository format", "Repository must be in workspace/slug format.")
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("%s/branch-restrictions/%s", base, state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to delete branch restrictions.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete branch restriction",
			fmt.Sprintf("Could not delete branch restriction %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing branch restriction by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// listToStrings converts a types.List of strings to a Go []string.
func listToStrings(ctx context.Context, list types.List) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}
	var result []string
	list.ElementsAs(ctx, &result, false)
	return result
}

// stringsToList converts a Go []string to a types.List of strings.
func stringsToList(_ context.Context, vals []string) types.List {
	if vals == nil {
		vals = []string{}
	}
	elems := make([]attr.Value, len(vals))
	for i, v := range vals {
		elems[i] = types.StringValue(v)
	}
	list, _ := types.ListValue(types.StringType, elems)
	return list
}
