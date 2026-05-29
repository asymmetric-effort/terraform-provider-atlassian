// Package space implements the atlassian_jira_space read-only data source.
//
// This data source reads Jira spaces (projects) by ID or key from the
// Atlassian Cloud REST API.
package space

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiSpace represents the JSON structure returned by the Atlassian project API.
type apiSpace struct {
	ID                 string `json:"id"`
	Key                string `json:"key"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	LeadAccountID      string `json:"leadAccountId,omitempty"`
	ProjectTypeKey     string `json:"projectTypeKey"`
	ProjectTemplateKey string `json:"projectTemplateKey,omitempty"`
	AvatarID           int64  `json:"avatarId,omitempty"`
	CategoryID         int64  `json:"categoryId,omitempty"`
	AssigneeType       string `json:"assigneeType,omitempty"`
	Self               string `json:"self"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	Key                types.String `tfsdk:"key"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	LeadAccountID      types.String `tfsdk:"lead_account_id"`
	SpaceType          types.String `tfsdk:"space_type"`
	ProjectTemplateKey types.String `tfsdk:"project_template_key"`
	AvatarID           types.Int64  `tfsdk:"avatar_id"`
	CategoryID         types.Int64  `tfsdk:"category_id"`
	AssigneeType       types.String `tfsdk:"assignee_type"`
	URL                types.String `tfsdk:"url"`
	SelfURL            types.String `tfsdk:"self_url"`
	BrowseURL          types.String `tfsdk:"browse_url"`
}

// DataSource implements the atlassian_jira_space data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_space"
}

// Schema defines the schema for the jira space data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira space (project) from Atlassian Cloud by ID, key, or name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the space. Provide id, key, or name.",
				Optional:    true,
				Computed:    true,
			},
			"key": schema.StringAttribute{
				Description: "The project key (e.g., PROJ). Provide id, key, or name.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the space. Provide id, key, or name.",
				Optional:    true,
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the space.",
				Computed:    true,
			},
			"lead_account_id": schema.StringAttribute{
				Description: "The account ID of the space lead.",
				Computed:    true,
			},
			"space_type": schema.StringAttribute{
				Description: "The type of space (\"classic\" or \"next-gen\").",
				Computed:    true,
			},
			"project_template_key": schema.StringAttribute{
				Description: "The project template key used when creating the space.",
				Computed:    true,
			},
			"avatar_id": schema.Int64Attribute{
				Description: "The ID of the avatar for the space.",
				Computed:    true,
			},
			"category_id": schema.Int64Attribute{
				Description: "The ID of the project category for the space.",
				Computed:    true,
			},
			"assignee_type": schema.StringAttribute{
				Description: "The default assignee type for the space. Either \"PROJECT_LEAD\" or \"UNASSIGNED\".",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "The URL of the space in Atlassian Cloud.",
				Computed:    true,
			},
			"self_url": schema.StringAttribute{
				Description: "The self URL of the space resource in the Atlassian API.",
				Computed:    true,
			},
			"browse_url": schema.StringAttribute{
				Description: "The browser-accessible URL of the space.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*atlassian.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	d.client = client
}

// projectTypeKeyToSpaceType converts the Atlassian API projectTypeKey to the user-facing space_type.
func projectTypeKeyToSpaceType(ptk string) string {
	if ptk == "software" {
		return "next-gen"
	}
	return "classic"
}

// Read retrieves space data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()
	hasKey := !config.Key.IsNull() && !config.Key.IsUnknown()
	hasName := !config.Name.IsNull() && !config.Name.IsUnknown()

	if !hasID && !hasKey && !hasName {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"At least one of id, key, or name must be specified to look up a Jira space.",
		)
		return
	}

	var space apiSpace

	if hasID || hasKey {
		identifier := config.ID.ValueString()
		if !hasID {
			identifier = config.Key.ValueString()
		}
		err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/project/%s", identifier), &space)
		if err != nil {
			if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
				resp.Diagnostics.AddError(
					"Space not found",
					fmt.Sprintf("Jira space %q not found. Verify the ID or key is correct.", identifier),
				)
				return
			}
			resp.Diagnostics.AddError(
				"Failed to read space",
				fmt.Sprintf("Could not read Jira space %q: %s", identifier, err.Error()),
			)
			return
		}
	} else {
		found, err := d.findSpaceByName(ctx, config.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to search spaces",
				fmt.Sprintf("Could not search for Jira space by name %q: %s", config.Name.ValueString(), err.Error()),
			)
			return
		}
		space = found
	}

	config.ID = types.StringValue(space.ID)
	config.Key = types.StringValue(space.Key)
	config.Name = types.StringValue(space.Name)
	config.Description = types.StringValue(space.Description)
	config.LeadAccountID = types.StringValue(space.LeadAccountID)
	config.SpaceType = types.StringValue(projectTypeKeyToSpaceType(space.ProjectTypeKey))
	config.ProjectTemplateKey = types.StringValue(space.ProjectTemplateKey)
	config.AvatarID = types.Int64Value(space.AvatarID)
	config.CategoryID = types.Int64Value(space.CategoryID)
	config.AssigneeType = types.StringValue(space.AssigneeType)
	config.URL = types.StringValue(space.Self)
	config.SelfURL = types.StringValue(space.Self)
	config.BrowseURL = types.StringValue(browseURL(space.Self, space.Key))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// findSpaceByName searches for a space by name using the project list API.
func (d *DataSource) findSpaceByName(ctx context.Context, name string) (apiSpace, error) {
	var spaces []json.RawMessage
	err := d.client.Get(ctx, "/rest/api/3/project", &spaces)
	if err != nil {
		return apiSpace{}, err
	}

	for _, raw := range spaces {
		var s apiSpace
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}

	return apiSpace{}, &atlassian.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("no Jira space found with name %q", name),
		Resource:   "jira_space",
		Action:     "read",
	}
}

// browseURL constructs a browser-accessible URL from the API self URL
// and the space key.
func browseURL(selfURL, key string) string {
	idx := strings.Index(selfURL, "/rest/api/")
	if idx < 0 {
		return ""
	}
	return selfURL[:idx] + "/browse/" + key
}
