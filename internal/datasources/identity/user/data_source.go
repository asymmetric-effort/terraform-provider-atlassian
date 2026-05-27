// Package user implements the atlassian_user read-only data source.
//
// This data source reads Atlassian Cloud user accounts by account ID
// or email address.
package user

import (
	"context"
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

// apiUser represents the JSON structure returned by the Atlassian user API.
type apiUser struct {
	AccountID   string `json:"accountId"`
	Email       string `json:"emailAddress"`
	DisplayName string `json:"displayName"`
	Active      bool   `json:"active"`
}

// apiUserSearchResponse represents the JSON structure for user search results.
type apiUserSearchResponse struct {
	AccountID   string `json:"accountId"`
	Email       string `json:"emailAddress"`
	DisplayName string `json:"displayName"`
	Active      bool   `json:"active"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	AccountID   types.String `tfsdk:"account_id"`
	Email       types.String `tfsdk:"email"`
	DisplayName types.String `tfsdk:"display_name"`
	Active      types.Bool   `tfsdk:"active"`
}

// DataSource implements the atlassian_user data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

// Schema defines the schema for the user data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an Atlassian Cloud user account by account ID or email address.",
		Attributes: map[string]schema.Attribute{
			"account_id": schema.StringAttribute{
				Description: "The unique account ID of the user. Exactly one of account_id or email must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"email": schema.StringAttribute{
				Description: "The email address of the user. Exactly one of account_id or email must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"display_name": schema.StringAttribute{
				Description: "The display name of the user.",
				Computed:    true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the user account is active.",
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

// Read retrieves user data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasAccountID := !config.AccountID.IsNull() && !config.AccountID.IsUnknown()
	hasEmail := !config.Email.IsNull() && !config.Email.IsUnknown()

	if !hasAccountID && !hasEmail {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"Exactly one of account_id or email must be specified to look up a user.",
		)
		return
	}

	var user apiUser
	var err error

	if hasAccountID {
		err = d.client.Get(ctx, fmt.Sprintf("/rest/api/3/user?accountId=%s", config.AccountID.ValueString()), &user)
	} else {
		user, err = d.findUserByEmail(ctx, config.Email.ValueString())
	}

	if err != nil {
		msg := err.Error()
		if isStatusCode(err, http.StatusNotFound) {
			if hasAccountID {
				msg = fmt.Sprintf("User with account ID %s not found. Verify the account ID is correct.", config.AccountID.ValueString())
			} else {
				msg = fmt.Sprintf("User with email %s not found. Verify the email address is correct.", config.Email.ValueString())
			}
		}
		resp.Diagnostics.AddError("Failed to read user", msg)
		return
	}

	config.AccountID = types.StringValue(user.AccountID)
	config.Email = types.StringValue(user.Email)
	config.DisplayName = types.StringValue(user.DisplayName)
	config.Active = types.BoolValue(user.Active)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// findUserByEmail searches for a user by email address using the user search API.
func (d *DataSource) findUserByEmail(ctx context.Context, email string) (apiUser, error) {
	var results []apiUserSearchResponse
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/user/search?query=%s", email), &results)
	if err != nil {
		return apiUser{}, err
	}

	// Search results by query string may return multiple matches.
	// Find the exact email match.
	for _, result := range results {
		if strings.EqualFold(result.Email, email) {
			return apiUser{
				AccountID:   result.AccountID,
				Email:       result.Email,
				DisplayName: result.DisplayName,
				Active:      result.Active,
			}, nil
		}
	}

	return apiUser{}, &atlassian.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("no user found with email %q", email),
		Resource:   "user",
		Action:     "read",
	}
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
