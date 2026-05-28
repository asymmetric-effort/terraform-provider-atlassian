// Package custom_domain implements the atlassian_jira_custom_email read-only data source.
//
// This data source reads Jira custom email addresses by ID or email address.
// It exposes all email attributes so operators can reference existing email
// configurations declaratively.
package custom_domain

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

// Ensure the EmailDataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &EmailDataSource{}

// apiEmailDS represents the JSON structure returned by the Atlassian email API.
type apiEmailDS struct {
	ID           string `json:"id"`
	EmailAddress string `json:"emailAddress"`
	DomainID     string `json:"domainId"`
	SpaceID      string `json:"spaceId,omitempty"`
	Active       bool   `json:"active"`
}

// EmailDataSourceModel describes the email data source data model.
type EmailDataSourceModel struct {
	ID           types.String `tfsdk:"id"`
	EmailAddress types.String `tfsdk:"email_address"`
	DomainID     types.String `tfsdk:"domain_id"`
	SpaceID      types.String `tfsdk:"space_id"`
	Active       types.Bool   `tfsdk:"active"`
}

// EmailDataSource implements the atlassian_jira_custom_email data source.
type EmailDataSource struct {
	client *atlassian.Client
}

// NewEmailDataSource returns a new EmailDataSource instance for provider registration.
func NewEmailDataSource() datasource.DataSource {
	return &EmailDataSource{}
}

// Metadata returns the data source type name.
func (d *EmailDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_custom_email"
}

// Schema defines the schema for the custom email data source.
func (d *EmailDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira custom email address by ID or email address.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the custom email. Exactly one of id or email_address must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"email_address": schema.StringAttribute{
				Description: "The email address. Exactly one of id or email_address must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"domain_id": schema.StringAttribute{
				Description: "The ID of the custom domain this email belongs to.",
				Computed:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The ID of the Jira space associated with this email address.",
				Computed:    true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the email address is active.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *EmailDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves custom email data from the Atlassian API.
func (d *EmailDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config EmailDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()
	hasEmail := !config.EmailAddress.IsNull() && !config.EmailAddress.IsUnknown()

	if !hasID && !hasEmail {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"Exactly one of id or email_address must be specified to look up a custom email.",
		)
		return
	}

	var email apiEmailDS
	var err error

	if hasID {
		err = d.client.Get(ctx, fmt.Sprintf("/rest/api/3/email/%s", config.ID.ValueString()), &email)
	} else {
		email, err = d.findEmailByAddress(ctx, config.EmailAddress.ValueString())
	}

	if err != nil {
		msg := err.Error()
		if isStatusCode(err, http.StatusNotFound) {
			if hasID {
				msg = fmt.Sprintf("Custom email with ID %s not found. Verify the ID is correct.", config.ID.ValueString())
			} else {
				msg = fmt.Sprintf("Custom email %q not found. Verify the email address is correct.", config.EmailAddress.ValueString())
			}
		}
		resp.Diagnostics.AddError("Failed to read custom email", msg)
		return
	}

	mapEmailDSAPIToModel(&email, &config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// findEmailByAddress searches for an email by address using the email list API.
func (d *EmailDataSource) findEmailByAddress(ctx context.Context, address string) (apiEmailDS, error) {
	var results []apiEmailDS
	err := d.client.Get(ctx, "/rest/api/3/email", &results)
	if err != nil {
		return apiEmailDS{}, err
	}

	for _, email := range results {
		if strings.EqualFold(email.EmailAddress, address) {
			return email, nil
		}
	}

	return apiEmailDS{}, &atlassian.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("no email found with address %q", address),
		Resource:   "email",
		Action:     "read",
	}
}

// mapEmailDSAPIToModel maps an API email response to the Terraform data source model.
func mapEmailDSAPIToModel(email *apiEmailDS, model *EmailDataSourceModel) {
	model.ID = types.StringValue(email.ID)
	model.EmailAddress = types.StringValue(email.EmailAddress)
	model.DomainID = types.StringValue(email.DomainID)
	model.Active = types.BoolValue(email.Active)
	if email.SpaceID != "" {
		model.SpaceID = types.StringValue(email.SpaceID)
	} else {
		model.SpaceID = types.StringNull()
	}
}
