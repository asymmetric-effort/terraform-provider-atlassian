// Package custom_domain implements the atlassian_jira_custom_domain managed resource.
//
// This resource manages Jira custom domains through the Atlassian Cloud
// REST API. It supports Create, Read, Delete, and ImportState operations.
// Domains are immutable; changing domain_name forces replacement.
//
// DNS record outputs (MX, TXT/SPF, DKIM, CNAME) are computed from the
// API response and surfaced as structured attributes so operators can
// configure their DNS providers declaratively.
package custom_domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
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

// apiDomainCreate represents the JSON body for creating a domain.
type apiDomainCreate struct {
	DomainName string `json:"domainName"`
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

// ResourceModel describes the resource data model.
type ResourceModel struct {
	ID                 types.String     `tfsdk:"id"`
	DomainName         types.String     `tfsdk:"domain_name"`
	Verified           types.Bool       `tfsdk:"verified"`
	VerificationStatus types.String     `tfsdk:"verification_status"`
	MXRecords          []MXRecordModel  `tfsdk:"mx_records"`
	TXTRecords         []DNSRecordModel `tfsdk:"txt_records"`
	DKIMRecords        []DNSRecordModel `tfsdk:"dkim_records"`
	CNAMERecords       []DNSRecordModel `tfsdk:"cname_records"`
}

// Resource implements the atlassian_jira_custom_domain managed resource.
type Resource struct {
	client *atlassian.Client
}

// NewResource returns a new Resource instance for provider registration.
func NewResource() resource.Resource {
	return &Resource{}
}

// Metadata returns the resource type name.
func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_jira_custom_domain"
}

// Schema defines the schema for the custom domain resource.
func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Jira custom domain. Domains are immutable; changing the domain name forces replacement.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The unique identifier of the custom domain, assigned by Atlassian.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"domain_name": schema.StringAttribute{
				Description: "The fully qualified domain name to register (e.g., mail.example.com). Changing this forces a new resource.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"verified": schema.BoolAttribute{
				Description: "Whether the domain has been verified via DNS records.",
				Computed:    true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"verification_status": schema.StringAttribute{
				Description: "The current verification status of the domain (e.g., pending, verified, failed).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"mx_records": schema.ListNestedBlock{
				Description: "MX DNS records required for email routing.",
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
				Description: "TXT DNS records (including SPF) required for domain verification.",
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
				Description: "DKIM CNAME DNS records required for email authentication.",
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
				Description: "CNAME DNS records required for domain verification.",
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

// Create provisions a new custom domain.
func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := apiDomainCreate{
		DomainName: plan.DomainName.ValueString(),
	}
	bodyBytes, _ := json.Marshal(body)

	var created apiDomain
	err := r.client.Post(ctx, "/rest/api/3/domain", bytes.NewReader(bodyBytes), &created)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusConflict:
				resp.Diagnostics.AddError(
					"Domain already registered",
					fmt.Sprintf("The domain %q is already registered. Each domain can only be registered once.", plan.DomainName.ValueString()),
				)
				return
			case http.StatusBadRequest:
				resp.Diagnostics.AddError(
					"Invalid domain name",
					fmt.Sprintf("The domain name %q is not valid. Provide a fully qualified domain name (e.g., mail.example.com).", plan.DomainName.ValueString()),
				)
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					"The authenticated user does not have permission to register custom domains. Ensure the service account has Jira admin privileges.",
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to create custom domain",
			fmt.Sprintf("Could not register domain %q: %s", plan.DomainName.ValueString(), err.Error()),
		)
		return
	}

	mapAPIToModel(&created, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read refreshes the Terraform state with the latest data from Atlassian.
func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var domain apiDomain
	err := r.client.Get(ctx, fmt.Sprintf("/rest/api/3/domain/%s", state.ID.ValueString()), &domain)
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok && apiErr.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Failed to read custom domain",
			fmt.Sprintf("Could not read domain with ID %q: %s. Verify the domain ID is correct and the domain has not been deleted.",
				state.ID.ValueString(), err.Error()),
		)
		return
	}

	mapAPIToModel(&domain, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is not supported for custom domains. Domains are immutable.
func (r *Resource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Update not supported",
		"Custom domains are immutable and cannot be updated. Change the domain_name to force replacement.",
	)
}

// Delete removes a custom domain.
func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.Delete(ctx, fmt.Sprintf("/rest/api/3/domain/%s", state.ID.ValueString()))
	if err != nil {
		if apiErr, ok := err.(*atlassian.APIError); ok {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				// Already deleted, nothing to do.
				return
			case http.StatusForbidden:
				resp.Diagnostics.AddError(
					"Permission denied",
					fmt.Sprintf("The authenticated user does not have permission to delete domain %q. "+
						"Ensure the service account has Jira admin privileges.", state.ID.ValueString()),
				)
				return
			}
		}
		resp.Diagnostics.AddError(
			"Failed to delete custom domain",
			fmt.Sprintf("Could not delete domain with ID %q: %s", state.ID.ValueString(), err.Error()),
		)
		return
	}
}

// ImportState imports an existing custom domain by ID.
func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// mapAPIToModel maps an API domain response to the Terraform resource model.
func mapAPIToModel(domain *apiDomain, model *ResourceModel) {
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
