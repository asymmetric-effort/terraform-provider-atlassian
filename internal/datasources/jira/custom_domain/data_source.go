// Package custom_domain implements the atlassian_jira_custom_domain read-only data source.
//
// This data source reads Jira custom domains by ID or domain name.
// It exposes the same DNS record outputs as the managed resource
// so that operators can reference DNS configuration from existing domains.
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

// Ensure the DataSource type satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &DataSource{}

// apiDNSRecord represents a single DNS record in the API response.
type apiDNSRecord struct {
	Host     string `json:"host"`
	Priority int    `json:"priority,omitempty"`
	Value    string `json:"value"`
}

// apiDomain represents the JSON structure returned by the Atlassian domain API.
type apiDomain struct {
	ID                 string         `json:"id"`
	DomainName         string         `json:"domainName"`
	Verified           bool           `json:"verified"`
	VerificationStatus string         `json:"verificationStatus"`
	MXRecords          []apiDNSRecord `json:"mxRecords"`
	TXTRecords         []apiDNSRecord `json:"txtRecords"`
	DKIMRecords        []apiDNSRecord `json:"dkimRecords"`
	CNAMERecords       []apiDNSRecord `json:"cnameRecords"`
}

// MXRecordModel represents a single MX DNS record in the Terraform state.
type MXRecordModel struct {
	Host     types.String `tfsdk:"host"`
	Priority types.Int64  `tfsdk:"priority"`
	Value    types.String `tfsdk:"value"`
}

// DNSRecordModel represents a single DNS record (TXT, DKIM, CNAME) in the Terraform state.
type DNSRecordModel struct {
	Host  types.String `tfsdk:"host"`
	Value types.String `tfsdk:"value"`
}

// DataSourceModel describes the data source data model.
type DataSourceModel struct {
	ID                 types.String     `tfsdk:"id"`
	DomainName         types.String     `tfsdk:"domain_name"`
	Verified           types.Bool       `tfsdk:"verified"`
	VerificationStatus types.String     `tfsdk:"verification_status"`
	MXRecords          []MXRecordModel  `tfsdk:"mx_records"`
	TXTRecords         []DNSRecordModel `tfsdk:"txt_records"`
	DKIMRecords        []DNSRecordModel `tfsdk:"dkim_records"`
	CNAMERecords       []DNSRecordModel `tfsdk:"cname_records"`
}

// DataSource implements the atlassian_jira_custom_domain data source.
type DataSource struct {
	client *atlassian.Client
}

// NewDataSource returns a new DataSource instance for provider registration.
func NewDataSource() datasource.DataSource {
	return &DataSource{}
}

// Metadata returns the data source type name.
func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_custom_domain"
}

// Schema defines the schema for the custom domain data source.
func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a Jira custom domain by ID or domain name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the custom domain. Exactly one of id or domain_name must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"domain_name": schema.StringAttribute{
				Description: "The fully qualified domain name. Exactly one of id or domain_name must be specified.",
				Optional:    true,
				Computed:    true,
			},
			"verified": schema.BoolAttribute{
				Description: "Whether the domain has been verified via DNS records.",
				Computed:    true,
			},
			"verification_status": schema.StringAttribute{
				Description: "The current verification status of the domain.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"mx_records": schema.ListNestedBlock{
				Description: "MX DNS records for email routing.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"host": schema.StringAttribute{
							Description: "The DNS host name for the MX record.",
							Computed:    true,
						},
						"priority": schema.Int64Attribute{
							Description: "The MX record priority.",
							Computed:    true,
						},
						"value": schema.StringAttribute{
							Description: "The MX record value (mail server).",
							Computed:    true,
						},
					},
				},
			},
			"txt_records": schema.ListNestedBlock{
				Description: "TXT DNS records (including SPF) for domain verification.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"host": schema.StringAttribute{
							Description: "The DNS host name for the TXT record.",
							Computed:    true,
						},
						"value": schema.StringAttribute{
							Description: "The TXT record value.",
							Computed:    true,
						},
					},
				},
			},
			"dkim_records": schema.ListNestedBlock{
				Description: "DKIM CNAME DNS records for email authentication.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"host": schema.StringAttribute{
							Description: "The DNS host name for the DKIM CNAME record.",
							Computed:    true,
						},
						"value": schema.StringAttribute{
							Description: "The DKIM CNAME record value.",
							Computed:    true,
						},
					},
				},
			},
			"cname_records": schema.ListNestedBlock{
				Description: "CNAME DNS records for domain verification.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"host": schema.StringAttribute{
							Description: "The DNS host name for the CNAME record.",
							Computed:    true,
						},
						"value": schema.StringAttribute{
							Description: "The CNAME record value.",
							Computed:    true,
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

// Read retrieves custom domain data from the Atlassian API.
func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config DataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := !config.ID.IsNull() && !config.ID.IsUnknown()
	hasDomainName := !config.DomainName.IsNull() && !config.DomainName.IsUnknown()

	if !hasID && !hasDomainName {
		resp.Diagnostics.AddError(
			"Missing required attribute",
			"Exactly one of id or domain_name must be specified to look up a custom domain.",
		)
		return
	}

	var domain apiDomain
	var err error

	if hasID {
		err = d.client.Get(ctx, fmt.Sprintf("/rest/api/3/domain/%s", config.ID.ValueString()), &domain)
	} else {
		domain, err = d.findDomainByName(ctx, config.DomainName.ValueString())
	}

	if err != nil {
		msg := err.Error()
		if isStatusCode(err, http.StatusNotFound) {
			if hasID {
				msg = fmt.Sprintf("Custom domain with ID %s not found. Verify the ID is correct.", config.ID.ValueString())
			} else {
				msg = fmt.Sprintf("Custom domain %q not found. Verify the domain name is correct.", config.DomainName.ValueString())
			}
		}
		resp.Diagnostics.AddError("Failed to read custom domain", msg)
		return
	}

	mapAPIToModel(&domain, &config)
	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)
}

// findDomainByName searches for a domain by name using the domain list API.
func (d *DataSource) findDomainByName(ctx context.Context, name string) (apiDomain, error) {
	var results []apiDomain
	err := d.client.Get(ctx, "/rest/api/3/domain", &results)
	if err != nil {
		return apiDomain{}, err
	}

	for _, domain := range results {
		if strings.EqualFold(domain.DomainName, name) {
			return domain, nil
		}
	}

	return apiDomain{}, &atlassian.APIError{
		StatusCode: http.StatusNotFound,
		Message:    fmt.Sprintf("no domain found with name %q", name),
		Resource:   "domain",
		Action:     "read",
	}
}

// isStatusCode checks whether an error is an APIError with the given HTTP status code.
// The caller must ensure err is non-nil before calling this function.
func isStatusCode(err error, code int) bool {
	msg := err.Error()
	expected := fmt.Sprintf("HTTP %d)", code)
	return strings.Contains(msg, expected)
}

// mapAPIToModel maps an API domain response to the Terraform data source model.
func mapAPIToModel(domain *apiDomain, model *DataSourceModel) {
	model.ID = types.StringValue(domain.ID)
	model.DomainName = types.StringValue(domain.DomainName)
	model.Verified = types.BoolValue(domain.Verified)
	model.VerificationStatus = types.StringValue(domain.VerificationStatus)

	model.MXRecords = make([]MXRecordModel, len(domain.MXRecords))
	for i, rec := range domain.MXRecords {
		model.MXRecords[i] = MXRecordModel{
			Host:     types.StringValue(rec.Host),
			Priority: types.Int64Value(int64(rec.Priority)),
			Value:    types.StringValue(rec.Value),
		}
	}

	model.TXTRecords = make([]DNSRecordModel, len(domain.TXTRecords))
	for i, rec := range domain.TXTRecords {
		model.TXTRecords[i] = DNSRecordModel{
			Host:  types.StringValue(rec.Host),
			Value: types.StringValue(rec.Value),
		}
	}

	model.DKIMRecords = make([]DNSRecordModel, len(domain.DKIMRecords))
	for i, rec := range domain.DKIMRecords {
		model.DKIMRecords[i] = DNSRecordModel{
			Host:  types.StringValue(rec.Host),
			Value: types.StringValue(rec.Value),
		}
	}

	model.CNAMERecords = make([]DNSRecordModel, len(domain.CNAMERecords))
	for i, rec := range domain.CNAMERecords {
		model.CNAMERecords[i] = DNSRecordModel{
			Host:  types.StringValue(rec.Host),
			Value: types.StringValue(rec.Value),
		}
	}
}
