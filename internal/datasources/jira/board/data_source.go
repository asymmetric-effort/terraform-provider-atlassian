// Package board implements the atlassian_jira_board read-only data source.
//
// This data source reads Jira boards by ID from the Atlassian Cloud REST API.
package board

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

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiColumnConfig represents a single column in the board column configuration.
type apiColumnConfig struct {
	Name      string   `json:"name"`
	StatusIDs []string `json:"statusIds,omitempty"`
}

// apiBoard represents the JSON structure returned by the Atlassian board API.
type apiBoard struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	SpaceID      string            `json:"spaceId"`
	FilterID     string            `json:"filterId,omitempty"`
	Self         string            `json:"self"`
	ColumnConfig []apiColumnConfig `json:"columnConfig,omitempty"`
}

// columnConfigObjectType is the attr.Type for the column config nested object.
var columnConfigObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"name":       types.StringType,
		"status_ids": types.ListType{ElemType: types.StringType},
	},
}

// columnConfigToState converts API column configs to the Terraform state list.
func columnConfigToState(ctx context.Context, configs []apiColumnConfig) types.List {
	if len(configs) == 0 {
		return types.ListNull(columnConfigObjectType)
	}
	var elems []attr.Value
	for _, c := range configs {
		var statusElems []attr.Value
		for _, s := range c.StatusIDs {
			statusElems = append(statusElems, types.StringValue(s))
		}
		var statusList types.List
		if len(statusElems) == 0 {
			statusList = types.ListNull(types.StringType)
		} else {
			statusList, _ = types.ListValue(types.StringType, statusElems)
		}
		obj, _ := types.ObjectValue(
			map[string]attr.Type{
				"name":       types.StringType,
				"status_ids": types.ListType{ElemType: types.StringType},
			},
			map[string]attr.Value{
				"name":       types.StringValue(c.Name),
				"status_ids": statusList,
			},
		)
		elems = append(elems, obj)
	}
	list, _ := types.ListValue(columnConfigObjectType, elems)
	_ = ctx
	return list
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Type         types.String `tfsdk:"type"`
	SpaceID      types.String `tfsdk:"space_id"`
	FilterID     types.String `tfsdk:"filter_id"`
	ColumnConfig types.List   `tfsdk:"column_config"`
}

// DataSource implements the atlassian_jira_board data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_board"
}

// Schema defines the schema for the jira board data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira board from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the board.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the board.",
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "The type of the board (scrum or kanban).",
				Computed:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the space (project) associated with this board.",
				Computed:    true,
			},
			"filter_id": schema.StringAttribute{
				Description: "The ID of the JQL filter this board is based on.",
				Computed:    true,
			},
			"column_config": schema.ListNestedAttribute{
				Description: "Column configuration for the board, defining columns and their mapped statuses.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "The name of the column.",
							Computed:    true,
						},
						"status_ids": schema.ListAttribute{
							Description: "List of status IDs mapped to this column.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
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

// Read retrieves board data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	identifier := config.ID.ValueString()

	var b apiBoard
	err := d.client.Get(ctx, fmt.Sprintf("/rest/agile/1.0/board/%s", identifier), &b)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Board not found",
				fmt.Sprintf("Jira board %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read board",
			fmt.Sprintf("Could not read Jira board %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(b.ID)
	config.Name = types.StringValue(b.Name)
	config.Type = types.StringValue(b.Type)
	config.SpaceID = types.StringValue(b.SpaceID)
	if b.FilterID != "" {
		config.FilterID = types.StringValue(b.FilterID)
	} else {
		config.FilterID = types.StringNull()
	}
	config.ColumnConfig = columnConfigToState(ctx, b.ColumnConfig)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
