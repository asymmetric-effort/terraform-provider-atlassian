// Package priority implements the atlassian_jira_priority_scheme read-only data source.
//
// This data source reads Jira priority schemes by ID from the Atlassian Cloud REST API.
package priority

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the SchemeDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &SchemeDataSource{}

// apiPriorityScheme represents the JSON structure returned by the Atlassian priority scheme API.
type apiPriorityScheme struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	PriorityIDs       []string `json:"priorityIds,omitempty"`
	DefaultPriorityID string   `json:"defaultPriorityId,omitempty"`
	Self              string   `json:"self"`
}

// SchemeDataSourceModel describes the priority scheme data source data model.
type SchemeDataSourceModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	PriorityIDs       types.List   `tfsdk:"priority_ids"`
	DefaultPriorityID types.String `tfsdk:"default_priority_id"`
}

// SchemeDataSource implements the atlassian_jira_priority_scheme data source.
type SchemeDataSource struct {
	client *atlassian.Client
}

// NewSchemeDataSource returns a new SchemeDataSource instance for provider registration.
func NewSchemeDataSource() datasource.DataSource {
	return &SchemeDataSource{}
}

// Metadata returns the data source type name.
func (d *SchemeDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_priority_scheme"
}

// Schema defines the schema for the jira priority scheme data source.
func (d *SchemeDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira priority scheme from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the priority scheme.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the priority scheme.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "A description of the priority scheme.",
				Computed:    true,
			},
			"priority_ids": schema.ListAttribute{
				Description: "Ordered list of priority IDs in the scheme, defining priority ordering.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"default_priority_id": schema.StringAttribute{
				Description: "The default priority ID for this scheme.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *SchemeDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// buildPriorityIDsList converts a string slice to a types.List of StringType values.
func buildPriorityIDsList(ids []string) types.List {
	if len(ids) == 0 {
		return types.ListValueMust(types.StringType, []attr.Value{})
	}
	elems := make([]attr.Value, len(ids))
	for i, id := range ids {
		elems[i] = types.StringValue(id)
	}
	return types.ListValueMust(types.StringType, elems)
}

// Read retrieves priority scheme data from the Atlassian API.
func (d *SchemeDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config SchemeDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var ps apiPriorityScheme
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/priorityscheme/%s", identifier), &ps)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Priority scheme not found",
				fmt.Sprintf("Jira priority scheme %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read priority scheme",
			fmt.Sprintf("Could not read Jira priority scheme %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(ps.ID)
	config.Name = types.StringValue(ps.Name)
	config.Description = types.StringValue(ps.Description)
	config.PriorityIDs = buildPriorityIDsList(ps.PriorityIDs)
	config.DefaultPriorityID = types.StringValue(ps.DefaultPriorityID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
