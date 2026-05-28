// Package page also implements the atlassian_confluence_content_restriction read-only data source.
//
// This data source reads content restrictions from the Atlassian Cloud REST API (v2).
package page

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the RestrictionDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &RestrictionDataSource{}

// apiRestriction represents the JSON structure for a content restriction.
type apiRestriction struct {
	Operation    string                   `json:"operation"`
	Restrictions apiRestrictionPrincipals `json:"restrictions,omitempty"`
}

// apiRestrictionPrincipals holds user and group restrictions.
type apiRestrictionPrincipals struct {
	User  apiRestrictionList `json:"user,omitempty"`
	Group apiRestrictionList `json:"group,omitempty"`
}

// apiRestrictionList holds a list of restriction entries.
type apiRestrictionList struct {
	Results []apiRestrictionEntry `json:"results,omitempty"`
}

// apiRestrictionEntry represents a single restriction entry.
type apiRestrictionEntry struct {
	Type      string `json:"type"`
	AccountID string `json:"accountId,omitempty"`
	ID        string `json:"id,omitempty"`
}

// RestrictionDataSourceModel describes the data source data model.
type RestrictionDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	ContentID     types.String `tfsdk:"content_id"`
	Operation     types.String `tfsdk:"operation"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
}

// RestrictionDataSource implements the atlassian_confluence_content_restriction data source.
type RestrictionDataSource struct {
	client *atlassian.Client
}

// NewRestrictionDataSource returns a new RestrictionDataSource instance for provider registration.
func NewRestrictionDataSource() datasource.DataSource {
	return &RestrictionDataSource{}
}

// Metadata returns the data source type name.
func (d *RestrictionDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_confluence_content_restriction"
}

// Schema defines the schema for the content restriction data source.
func (d *RestrictionDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a content restriction from a Confluence page in Atlassian Cloud.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The composite identifier (content_id/operation/principal_type/principal_id).",
				Computed:    true,
			},
			"content_id": schema.StringAttribute{
				Description: "The ID of the content (page).",
				Required:    true,
			},
			"operation": schema.StringAttribute{
				Description: "The operation to look up (\"read\" or \"update\").",
				Required:    true,
			},
			"principal_type": schema.StringAttribute{
				Description: "The type of principal (\"user\" or \"group\").",
				Required:    true,
			},
			"principal_id": schema.StringAttribute{
				Description: "The ID of the principal.",
				Required:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *RestrictionDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves content restriction data from the Atlassian API.
func (d *RestrictionDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config RestrictionDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	contentID := config.ContentID.ValueString()
	operation := config.Operation.ValueString()
	principalType := config.PrincipalType.ValueString()
	principalID := config.PrincipalID.ValueString()

	var restrictions []apiRestriction
	err := d.client.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restriction", contentID), &restrictions)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Content not found",
				fmt.Sprintf("Content %q not found.", contentID),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read content restrictions",
			fmt.Sprintf("Could not read restrictions for content %q: %s", contentID, err.Error()),
		)
		return
	}

	found := false
	for _, restriction := range restrictions {
		if restriction.Operation != operation {
			continue
		}
		if principalType == "user" {
			for _, u := range restriction.Restrictions.User.Results {
				if u.AccountID == principalID {
					found = true
					break
				}
			}
		} else {
			for _, g := range restriction.Restrictions.Group.Results {
				if g.ID == principalID {
					found = true
					break
				}
			}
		}
	}

	if !found {
		resp.Diagnostics.AddError(
			"Restriction not found",
			fmt.Sprintf("No %s restriction found for %s %q on content %q.", operation, principalType, principalID, contentID),
		)
		return
	}

	config.ID = types.StringValue(fmt.Sprintf("%s/%s/%s/%s", contentID, operation, principalType, principalID))

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
