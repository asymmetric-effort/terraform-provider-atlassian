// Package pipeline implements the atlassian_bitbucket_pipeline managed resource.
//
// This resource manages Bitbucket pipeline configuration through the Atlassian
// Cloud REST API. It supports full CRUD operations and state import
// via repository identifier.
package pipeline

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

// apiPipelineVariable represents a single pipeline variable in the API.
type apiPipelineVariable struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Secured bool   `json:"secured"`
}

// apiPipeline represents the JSON structure returned by the Bitbucket pipelines_config API.
type apiPipeline struct {
	UUID      string                `json:"uuid,omitempty"`
	Enabled   bool                  `json:"enabled"`
	Variables []apiPipelineVariable `json:"variables,omitempty"`
}

// variableObjectType is the attr.Type for the pipeline variable nested object.
var variableObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"key":     types.StringType,
		"value":   types.StringType,
		"secured": types.BoolType,
	},
}

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Repository types.String `tfsdk:"repository"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Variables  types.List   `tfsdk:"variables"`
}

// Resource implements the atlassian_bitbucket_pipeline managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bitbucket_pipeline"
}

// Schema defines the schema for the bitbucket pipeline resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Bitbucket pipeline configuration in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the pipeline configuration (same as repository).",
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
			"enabled": schema.BoolAttribute{
				Description: "Whether pipelines are enabled for this repository.",
				Optional:    true,
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"variables": schema.ListNestedAttribute{
				Description: "Pipeline variables for this repository.",
				Optional:    true,
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Description: "The name of the pipeline variable.",
							Required:    true,
						},
						"value": schema.StringAttribute{
							Description: "The value of the pipeline variable.",
							Required:    true,
						},
						"secured": schema.BoolAttribute{
							Description: "Whether the variable value is secured (masked in logs).",
							Optional:    true,
							Computed:    true,
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

// variablesFromPlan converts the Terraform variables list to API format.
func variablesFromPlan(ctx context.Context, vars types.List) []apiPipelineVariable {
	if vars.IsNull() || vars.IsUnknown() {
		return nil
	}
	var result []apiPipelineVariable
	var elements []types.Object
	vars.ElementsAs(ctx, &elements, false)
	for _, elem := range elements {
		attrs := elem.Attributes()
		v := apiPipelineVariable{
			Key:   attrs["key"].(types.String).ValueString(),
			Value: attrs["value"].(types.String).ValueString(),
		}
		if secured, ok := attrs["secured"].(types.Bool); ok && !secured.IsNull() && !secured.IsUnknown() {
			v.Secured = secured.ValueBool()
		}
		result = append(result, v)
	}
	return result
}

// variablesToState converts API pipeline variables to the Terraform state list.
func variablesToState(ctx context.Context, vars []apiPipelineVariable) types.List {
	if len(vars) == 0 {
		return types.ListNull(variableObjectType)
	}
	var elems []attr.Value
	for _, v := range vars {
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"key":     types.StringType,
				"value":   types.StringType,
				"secured": types.BoolType,
			},
			map[string]attr.Value{
				"key":     types.StringValue(v.Key),
				"value":   types.StringValue(v.Value),
				"secured": types.BoolValue(v.Secured),
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(variableObjectType, elems)
	return list
}

// repoPath builds the API path prefix for a repository.
func repoPath(repository string) string {
	parts := strings.SplitN(repository, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return fmt.Sprintf("/2.0/repositories/%s/%s", parts[0], parts[1])
}

// Create provisions a new Bitbucket pipeline configuration.
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

	enabled := true
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		enabled = plan.Enabled.ValueBool()
	}

	body := apiPipeline{
		Enabled:   enabled,
		Variables: variablesFromPlan(ctx, plan.Variables),
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiPipeline
	err := r.client.Put(ctx, base+"/pipelines_config", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to configure pipelines.",
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
			"Failed to create pipeline configuration",
			fmt.Sprintf("Could not create pipeline configuration: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(plan.Repository.ValueString())
	plan.Enabled = types.BoolValue(created.Enabled)
	plan.Variables = variablesToState(ctx, created.Variables)

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

	var pipeline apiPipeline
	err := r.client.Get(ctx, base+"/pipelines_config", &pipeline)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read pipeline configuration",
			fmt.Sprintf("Could not read pipeline configuration: %s", err.Error()),
		)
		return
	}

	state.ID = types.StringValue(state.Repository.ValueString())
	state.Enabled = types.BoolValue(pipeline.Enabled)
	state.Variables = variablesToState(ctx, pipeline.Variables)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update modifies an existing Bitbucket pipeline configuration.
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

	enabled := true
	if !plan.Enabled.IsNull() && !plan.Enabled.IsUnknown() {
		enabled = plan.Enabled.ValueBool()
	}

	body := apiPipeline{
		Enabled:   enabled,
		Variables: variablesFromPlan(ctx, plan.Variables),
	}
	bodyBytes, _ := json.Marshal(body)

	var updated apiPipeline
	err := r.client.Put(ctx, base+"/pipelines_config", bytes.NewReader(bodyBytes), &updated)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				resp.Diagnostics.AddError(
					"Repository not found",
					fmt.Sprintf("Repository %q not found. It may have been deleted outside of Terraform.", state.Repository.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update pipeline configuration.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to update pipeline configuration",
			fmt.Sprintf("Could not update pipeline configuration: %s", err.Error()),
		)
		return
	}

	plan.ID = types.StringValue(state.Repository.ValueString())
	plan.Enabled = types.BoolValue(updated.Enabled)
	plan.Variables = variablesToState(ctx, updated.Variables)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete removes a Bitbucket pipeline configuration (disables pipelines).
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

	// Disable pipelines on delete rather than truly deleting
	body := apiPipeline{Enabled: false}
	bodyBytes, _ := json.Marshal(body)

	err := r.client.Put(ctx, base+"/pipelines_config", bytes.NewReader(bodyBytes), nil)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to update pipeline configuration.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to disable pipeline configuration",
			fmt.Sprintf("Could not disable pipeline configuration: %s", err.Error()),
		)
		return
	}
}

// ImportState imports an existing pipeline configuration by repository.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
