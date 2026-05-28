// Package deployment implements the atlassian_bitbucket_deployment managed resource.
//
// This resource manages Bitbucket deployment environments through the Atlassian
// Cloud REST API. It supports full CRUD operations and state import
// via deployment environment UUID.
package deployment

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

// apiEnvironmentType represents the nested environment_type in the Bitbucket API.
type apiEnvironmentType struct {
	Name string `json:"name"`
}

// apiRestriction represents a single restriction entry in the Bitbucket API.
type apiRestriction struct {
	Type    string `json:"type"`
	Pattern string `json:"pattern,omitempty"`
}

// apiDeployment represents the JSON structure returned by the Bitbucket environments API.
type apiDeployment struct {
	UUID            string             `json:"uuid"`
	Name            string             `json:"name"`
	EnvironmentType apiEnvironmentType `json:"environment_type"`
	Lock            *bool              `json:"lock,omitempty"`
	Restrictions    []apiRestriction   `json:"restrictions,omitempty"`
}

// apiDeploymentCreate represents the JSON body for creating a deployment environment.
type apiDeploymentCreate struct {
	Name            string             `json:"name"`
	EnvironmentType apiEnvironmentType `json:"environment_type"`
	Lock            *bool              `json:"lock,omitempty"`
	Restrictions    []apiRestriction   `json:"restrictions,omitempty"`
}

// RestrictionModel describes a single restriction entry in the Terraform model.
type RestrictionModel struct {
	Type    string `tfsdk:"type"`
	Pattern string `tfsdk:"pattern"`
}

// restrictionObjectType is the attr.Type for the restriction nested object.
var restrictionObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"type":    types.StringType,
		"pattern": types.StringType,
	},
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Repository      types.String `tfsdk:"repository"`
	Name            types.String `tfsdk:"name"`
	EnvironmentType types.String `tfsdk:"environment_type"`
	Lock            types.Bool   `tfsdk:"lock"`
	Restrictions    types.List   `tfsdk:"restrictions"`
}

// Resource implements the atlassian_bitbucket_deployment managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_deployment"
}

// Schema defines the schema for the bitbucket deployment resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Bitbucket deployment environment in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier (UUID) of the deployment environment, assigned by Bitbucket.",
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
			"name": schema.StringAttribute{
				Description: "The name of the deployment environment.",
				Required:    true,
			},
			"environment_type": schema.StringAttribute{
				Description: "The type of deployment environment. Must be one of: test, staging, production.",
				Required:    true,
			},
			"lock": schema.BoolAttribute{
				Description: "Whether the deployment environment is locked.",
				Optional:    true,
				Computed:    true,
			},
			"restrictions": schema.ListNestedAttribute{
				Description: "Restrictions applied to the deployment environment.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Description: "The type of restriction (e.g., branch_restriction, admin_only).",
							Required:    true,
						},
						"pattern": schema.StringAttribute{
							Description: "The pattern for the restriction (e.g., a branch name pattern).",
							Optional:    true,
						},
					},
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

// repoPath builds the API path prefix for a repository.
func repoPath(repository string) string {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("/2.0/repositories/%s/%s", parts[0], parts[1])
}

// Create provisions a new Bitbucket deployment environment.
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

	body := apiDeploymentCreate{
		Name:            plan.Name.ValueString(),
		EnvironmentType: apiEnvironmentType{Name: plan.EnvironmentType.ValueString()},
	}
	if !plan.Lock.IsNull() && !plan.Lock.IsUnknown() {
		v := plan.Lock.ValueBool()
		body.Lock = &v
	}
	body.Restrictions = restrictionsFromPlan(ctx, plan.Restrictions)
	bodyBytes, _ := json.Marshal(body)

	var created apiDeployment
	err := r.client.Post(ctx, base+"/environments", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to create deployment environments.",
				)
				return
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Repository not found",
					fmt.Sprintf("Repository %q not found. Verify the workspace/slug is correct.", plan.Repository.ValueString()),
				)
				return
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Duplicate deployment environment",
					fmt.Sprintf("A deployment environment named %q already exists in this repository.", plan.Name.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create deployment environment",
			fmt.Sprintf("Could not create deployment environment: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(created.UUID)
	plan.Name = types.StringValue(created.Name)
	plan.EnvironmentType = types.StringValue(created.EnvironmentType.Name)
	plan.Lock = lockToState(created.Lock)
	plan.Restrictions = restrictionsToState(ctx, created.Restrictions)

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

	var env apiDeployment
	err := r.client.Get(ctx, fmt.Sprintf("%s/environments/%s", base, state.ID.ValueString()), &env)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read deployment environment",
			fmt.Sprintf("Could not read deployment environment %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	state.ID = types.StringValue(env.UUID)
	state.Name = types.StringValue(env.Name)
	state.EnvironmentType = types.StringValue(env.EnvironmentType.Name)
	state.Lock = lockToState(env.Lock)
	state.Restrictions = restrictionsToState(ctx, env.Restrictions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Bitbucket deployment environment.
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

	body := apiDeploymentCreate{
		Name:            plan.Name.ValueString(),
		EnvironmentType: apiEnvironmentType{Name: plan.EnvironmentType.ValueString()},
	}
	if !plan.Lock.IsNull() && !plan.Lock.IsUnknown() {
		v := plan.Lock.ValueBool()
		body.Lock = &v
	}
	body.Restrictions = restrictionsFromPlan(ctx, plan.Restrictions)
	bodyBytes, _ := json.Marshal(body)

	var updated apiDeployment
	err := r.client.Put(ctx, fmt.Sprintf("%s/environments/%s", base, state.ID.ValueString()), bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Deployment environment not found",
					fmt.Sprintf("Deployment environment %q not found. It may have been deleted outside of Terraform.", state.ID.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update deployment environments.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update deployment environment",
			fmt.Sprintf("Could not update deployment environment %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(updated.UUID)
	plan.Name = types.StringValue(updated.Name)
	plan.EnvironmentType = types.StringValue(updated.EnvironmentType.Name)
	plan.Lock = lockToState(updated.Lock)
	plan.Restrictions = restrictionsToState(ctx, updated.Restrictions)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Bitbucket deployment environment.
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

	err := r.client.Delete(ctx, fmt.Sprintf("%s/environments/%s", base, state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to delete deployment environments.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete deployment environment",
			fmt.Sprintf("Could not delete deployment environment %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing deployment environment by UUID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// lockToState converts an API lock pointer to a Terraform Bool value.
func lockToState(lock *bool) types.Bool {
	if lock == nil {
		return types.BoolValue(false)
	}
	return types.BoolValue(*lock)
}

// restrictionsFromPlan converts the Terraform restrictions list to API format.
func restrictionsFromPlan(ctx context.Context, restrictions types.List) []apiRestriction {
	if restrictions.IsNull() || restrictions.IsUnknown() {
		return nil
	}
	var models []RestrictionModel
	restrictions.ElementsAs(ctx, &models, false)
	var result []apiRestriction
	for _, m := range models {
		result = append(result, apiRestriction{
			Type:    m.Type,
			Pattern: m.Pattern,
		})
	}
	return result
}

// restrictionsToState converts API restrictions to the Terraform state list.
func restrictionsToState(ctx context.Context, restrictions []apiRestriction) types.List {
	if len(restrictions) == 0 {
		return types.ListNull(restrictionObjectType)
	}
	var elems []attr.Value
	for _, r := range restrictions {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"type":    types.StringType,
				"pattern": types.StringType,
			},
			map[string]attr.Value{
				"type":    types.StringValue(r.Type),
				"pattern": types.StringValue(r.Pattern),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(restrictionObjectType, elems)
	return list
}
