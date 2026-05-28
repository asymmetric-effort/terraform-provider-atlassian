// Package mailhandler implements the atlassian_jira_incoming_mail_handler and
// atlassian_jira_outgoing_mail_handler read-only data sources.
//
// These data sources read Jira mail handlers by ID from the
// Atlassian Cloud REST API.
package mailhandler

import (
	"context"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the data source types satisfy the datasource.DataSource interface.
var (
	_ datasource.DataSource = &IncomingDataSource{}
	_ datasource.DataSource = &OutgoingDataSource{}
)

// apiIncomingHandler represents the JSON structure returned by the Atlassian incoming mail handler API.
type apiIncomingHandler struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Server      string `json:"server"`
	Port        int64  `json:"port"`
	Protocol    string `json:"protocol"`
	Username    string `json:"username"`
	Folder      string `json:"folder,omitempty"`
	SpaceID     string `json:"spaceId,omitempty"`
	IssueTypeID string `json:"issueTypeId,omitempty"`
}

// apiOutgoingHandler represents the JSON structure returned by the Atlassian outgoing mail handler API.
type apiOutgoingHandler struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	FromAddress string `json:"fromAddress"`
	Prefix      string `json:"prefix,omitempty"`
	SMTPHost    string `json:"smtpHost"`
	SMTPPort    int64  `json:"smtpPort"`
	Protocol    string `json:"protocol,omitempty"`
	Username    string `json:"username,omitempty"`
	TLS         bool   `json:"tls"`
}

// IncomingDataSourceModel describes the incoming mail handler data source model.
type IncomingDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Server      types.String `tfsdk:"server"`
	Port        types.Int64  `tfsdk:"port"`
	Protocol    types.String `tfsdk:"protocol"`
	Username    types.String `tfsdk:"username"`
	Folder      types.String `tfsdk:"folder"`
	SpaceID     types.String `tfsdk:"space_id"`
	IssueTypeID types.String `tfsdk:"issue_type_id"`
}

// OutgoingDataSourceModel describes the outgoing mail handler data source model.
type OutgoingDataSourceModel struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	FromAddress types.String `tfsdk:"from_address"`
	Prefix      types.String `tfsdk:"prefix"`
	SMTPHost    types.String `tfsdk:"smtp_host"`
	SMTPPort    types.Int64  `tfsdk:"smtp_port"`
	Protocol    types.String `tfsdk:"protocol"`
	Username    types.String `tfsdk:"username"`
	TLS         types.Bool   `tfsdk:"tls"`
}

// ==================== INCOMING DATA SOURCE ====================

// IncomingDataSource implements the atlassian_jira_incoming_mail_handler data source.
type IncomingDataSource struct {
	client *atlassian.Client
}

// NewIncomingDataSource returns a new IncomingDataSource instance for provider registration.
func NewIncomingDataSource() datasource.DataSource {
	return &IncomingDataSource{}
}

// Metadata returns the data source type name.
func (d *IncomingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_incoming_mail_handler"
}

// Schema defines the schema for the incoming mail handler data source.
func (d *IncomingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira incoming mail handler from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the incoming mail handler.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the incoming mail handler.",
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the incoming mail handler is enabled.",
				Computed:    true,
			},
			"server": schema.StringAttribute{
				Description: "The mail server hostname.",
				Computed:    true,
			},
			"port": schema.Int64Attribute{
				Description: "The mail server port number.",
				Computed:    true,
			},
			"protocol": schema.StringAttribute{
				Description: "The mail protocol (\"IMAP\" or \"POP3\").",
				Computed:    true,
			},
			"username": schema.StringAttribute{
				Description: "The username for mail server authentication.",
				Computed:    true,
			},
			"folder": schema.StringAttribute{
				Description: "The mail folder to monitor.",
				Computed:    true,
			},
			"space_id": schema.StringAttribute{
				Description: "The Jira space ID for creating issues from incoming mail.",
				Computed:    true,
			},
			"issue_type_id": schema.StringAttribute{
				Description: "The issue type ID for creating issues from incoming mail.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *IncomingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves incoming mail handler data from the Atlassian API.
func (d *IncomingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config IncomingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.ID.IsNull() || config.ID.IsUnknown() {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"The id attribute must be specified to look up a Jira incoming mail handler.",
		)
		return
	}

	identifier := config.ID.ValueString()

	var handler apiIncomingHandler
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/mailhandler/incoming/%s", identifier), &handler)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Incoming mail handler not found",
				fmt.Sprintf("Jira incoming mail handler %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read incoming mail handler",
			fmt.Sprintf("Could not read Jira incoming mail handler %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(handler.ID)
	config.Name = types.StringValue(handler.Name)
	config.Enabled = types.BoolValue(handler.Enabled)
	config.Server = types.StringValue(handler.Server)
	config.Port = types.Int64Value(handler.Port)
	config.Protocol = types.StringValue(handler.Protocol)
	config.Username = types.StringValue(handler.Username)
	config.Folder = types.StringValue(handler.Folder)
	config.SpaceID = types.StringValue(handler.SpaceID)
	config.IssueTypeID = types.StringValue(handler.IssueTypeID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// ==================== OUTGOING DATA SOURCE ====================

// OutgoingDataSource implements the atlassian_jira_outgoing_mail_handler data source.
type OutgoingDataSource struct {
	client *atlassian.Client
}

// NewOutgoingDataSource returns a new OutgoingDataSource instance for provider registration.
func NewOutgoingDataSource() datasource.DataSource {
	return &OutgoingDataSource{}
}

// Metadata returns the data source type name.
func (d *OutgoingDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_outgoing_mail_handler"
}

// Schema defines the schema for the outgoing mail handler data source.
func (d *OutgoingDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira outgoing mail handler from Atlassian Cloud by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the outgoing mail handler.",
				Required:    true,
			},
			"name": schema.StringAttribute{
				Description: "The display name of the outgoing mail handler.",
				Computed:    true,
			},
			"from_address": schema.StringAttribute{
				Description: "The email address used as the From address for outgoing mail.",
				Computed:    true,
			},
			"prefix": schema.StringAttribute{
				Description: "The subject line prefix for outgoing mail.",
				Computed:    true,
			},
			"smtp_host": schema.StringAttribute{
				Description: "The SMTP server hostname.",
				Computed:    true,
			},
			"smtp_port": schema.Int64Attribute{
				Description: "The SMTP server port number.",
				Computed:    true,
			},
			"protocol": schema.StringAttribute{
				Description: "The mail protocol.",
				Computed:    true,
			},
			"username": schema.StringAttribute{
				Description: "The username for SMTP authentication.",
				Computed:    true,
			},
			"tls": schema.BoolAttribute{
				Description: "Whether TLS is used for the SMTP connection.",
				Computed:    true,
			},
		},
	}
}

// Configure sets the provider-configured client on the data source.
func (d *OutgoingDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

// Read retrieves outgoing mail handler data from the Atlassian API.
func (d *OutgoingDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config OutgoingDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.ID.IsNull() || config.ID.IsUnknown() {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"The id attribute must be specified to look up a Jira outgoing mail handler.",
		)
		return
	}

	identifier := config.ID.ValueString()

	var handler apiOutgoingHandler
	err := d.client.Get(ctx, fmt.Sprintf("/rest/api/3/mailhandler/outgoing/%s", identifier), &handler)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.Diagnostics.AddError(
				"Outgoing mail handler not found",
				fmt.Sprintf("Jira outgoing mail handler %q not found. Verify the ID is correct.", identifier),
			)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read outgoing mail handler",
			fmt.Sprintf("Could not read Jira outgoing mail handler %q: %s", identifier, err.Error()),
		)
		return
	}

	config.ID = types.StringValue(handler.ID)
	config.Name = types.StringValue(handler.Name)
	config.FromAddress = types.StringValue(handler.FromAddress)
	config.Prefix = types.StringValue(handler.Prefix)
	config.SMTPHost = types.StringValue(handler.SMTPHost)
	config.SMTPPort = types.Int64Value(handler.SMTPPort)
	config.Protocol = types.StringValue(handler.Protocol)
	config.Username = types.StringValue(handler.Username)
	config.TLS = types.BoolValue(handler.TLS)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}
