// Package provider implements the Atlassian OpenTofu/Terraform provider.
package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure AtlassianProvider satisfies the provider.Provider interface.
var _ provider.Provider = &AtlassianProvider{}

// AtlassianProvider defines the provider implementation.
type AtlassianProvider struct {
	version string
}

// AtlassianProviderModel describes the provider data model.
type AtlassianProviderModel struct {
	URL               types.String `tfsdk:"url"`
	Username          types.String `tfsdk:"username"`
	APIToken          types.String `tfsdk:"api_token"`
	OAuthClientID     types.String `tfsdk:"oauth_client_id"`
	OAuthClientSecret types.String `tfsdk:"oauth_client_secret"`
	OAuthRefreshToken types.String `tfsdk:"oauth_refresh_token"`
	RequestTimeout    types.String `tfsdk:"request_timeout"`
	MaxRetries        types.Int64  `tfsdk:"max_retries"`
	RetryWaitMin      types.String `tfsdk:"retry_wait_min"`
	RetryWaitMax      types.String `tfsdk:"retry_wait_max"`
}

// New returns a function that creates a new provider instance.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &AtlassianProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name.
func (p *AtlassianProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "atlassian"
	resp.Version = p.version
}

// Schema defines the provider-level schema.
func (p *AtlassianProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Atlassian provider enables declarative management of Atlassian Cloud resources.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "Atlassian Cloud site URL (e.g., https://example.atlassian.net). May be set via ATLASSIAN_URL environment variable.",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Service account email for API token authentication. May be set via ATLASSIAN_USERNAME environment variable.",
				Optional:    true,
			},
			"api_token": schema.StringAttribute{
				Description: "API token for authentication. May be set via ATLASSIAN_API_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"oauth_client_id": schema.StringAttribute{
				Description: "OAuth 2.0 client ID. May be set via ATLASSIAN_OAUTH_CLIENT_ID environment variable.",
				Optional:    true,
			},
			"oauth_client_secret": schema.StringAttribute{
				Description: "OAuth 2.0 client secret. May be set via ATLASSIAN_OAUTH_CLIENT_SECRET environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"oauth_refresh_token": schema.StringAttribute{
				Description: "OAuth 2.0 refresh token for three-legged auth. May be set via ATLASSIAN_OAUTH_REFRESH_TOKEN environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"request_timeout": schema.StringAttribute{
				Description: "Maximum time for a single API request (e.g., 30s, 1m). Defaults to 30s.",
				Optional:    true,
			},
			"max_retries": schema.Int64Attribute{
				Description: "Maximum number of retry attempts on HTTP 429/503 responses. Defaults to 5.",
				Optional:    true,
			},
			"retry_wait_min": schema.StringAttribute{
				Description: "Minimum wait between retries (e.g., 1s). Defaults to 1s.",
				Optional:    true,
			},
			"retry_wait_max": schema.StringAttribute{
				Description: "Maximum wait between retries (e.g., 30s). Defaults to 30s.",
				Optional:    true,
			},
		},
	}
}

// Configure prepares the Atlassian API client for data sources and resources.
func (p *AtlassianProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config AtlassianProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Configuration validation and client creation will be implemented
	// in objectives #7, #8, #9, and #10.
}

// Resources defines the resources implemented in the provider.
func (p *AtlassianProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

// DataSources defines the data sources implemented in the provider.
func (p *AtlassianProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}
