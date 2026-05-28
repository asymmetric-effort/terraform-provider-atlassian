// Package provider implements the Atlassian OpenTofu/Terraform provider.
package provider

import (
	"context"
	"os"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	bbrepodatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/bitbucket/repository"
	confluencepagedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/page"
	confluencespacedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/space"
	confluencespacepermdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/space"
	confluencetemplatedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/confluence/template"
	groupdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/group"
	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	userds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/user"
	automationdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/automation"
	boarddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/board"
	customdomainds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/custom_domain"
	customfielddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/custom_field"
	dashboarddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/dashboard"
	issuetypedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/issue_type"
	mailhandlerdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/mail_handler"
	notificationschemeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/notification_scheme"
	permissionschemeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/permission_scheme"
	prioritydatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/priority"
	screendatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/screen"
	securityschemeds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/security_scheme"
	spacedatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/space"
	workflowdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/jira/workflow"
	bbreporesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/bitbucket/repository"
	confluencepageresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/page"
	confluencespacepermresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/space"
	confluencespaceresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/space"
	confluencetemplateresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/confluence/template"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	userrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/user"
	automationresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/automation"
	boardresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/board"
	customdomainrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_domain"
	customfieldresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/custom_field"
	dashboardresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/dashboard"
	issuetyperesource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/issue_type"
	mailhandlerresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/mail_handler"
	notificationschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/notification_scheme"
	permissionschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/permission_scheme"
	priorityresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/priority"
	screenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/screen"
	securityschemers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/security_scheme"
	spaceresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/space"
	workflowresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/jira/workflow"
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

	// Resolve values with environment variable fallbacks
	url := stringValueOrEnv(config.URL, "ATLASSIAN_URL")
	username := stringValueOrEnv(config.Username, "ATLASSIAN_USERNAME")
	apiToken := stringValueOrEnv(config.APIToken, "ATLASSIAN_API_TOKEN")
	oauthClientID := stringValueOrEnv(config.OAuthClientID, "ATLASSIAN_OAUTH_CLIENT_ID")
	oauthClientSecret := stringValueOrEnv(config.OAuthClientSecret, "ATLASSIAN_OAUTH_CLIENT_SECRET")
	oauthRefreshToken := stringValueOrEnv(config.OAuthRefreshToken, "ATLASSIAN_OAUTH_REFRESH_TOKEN")

	// Build client config with retry parameters
	clientConfig := atlassian.DefaultConfig()
	clientConfig.BaseURL = url

	if !config.RequestTimeout.IsNull() && !config.RequestTimeout.IsUnknown() {
		d, err := time.ParseDuration(config.RequestTimeout.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid request_timeout",
				"Could not parse request_timeout as a duration (e.g., 30s, 1m): "+err.Error())
			return
		}
		clientConfig.RequestTimeout = d
	}
	if !config.MaxRetries.IsNull() && !config.MaxRetries.IsUnknown() {
		clientConfig.MaxRetries = int(config.MaxRetries.ValueInt64())
	}
	if !config.RetryWaitMin.IsNull() && !config.RetryWaitMin.IsUnknown() {
		d, err := time.ParseDuration(config.RetryWaitMin.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid retry_wait_min",
				"Could not parse retry_wait_min as a duration (e.g., 1s, 500ms): "+err.Error())
			return
		}
		clientConfig.RetryWaitMin = d
	}
	if !config.RetryWaitMax.IsNull() && !config.RetryWaitMax.IsUnknown() {
		d, err := time.ParseDuration(config.RetryWaitMax.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid retry_wait_max",
				"Could not parse retry_wait_max as a duration (e.g., 30s, 1m): "+err.Error())
			return
		}
		clientConfig.RetryWaitMax = d
	}

	// Determine authentication method
	var auth atlassian.Authenticator
	var authErr error

	hasAPIToken := username != "" || apiToken != ""
	hasOAuthRefresh := oauthRefreshToken != ""
	hasOAuthClientCreds := oauthClientID != "" && oauthClientSecret != "" && !hasOAuthRefresh

	switch {
	case hasAPIToken:
		auth, authErr = atlassian.NewTokenAuthenticator(username, apiToken)
	case hasOAuthRefresh:
		auth, authErr = atlassian.NewOAuthRefreshAuthenticator(oauthClientID, oauthClientSecret, oauthRefreshToken)
	case hasOAuthClientCreds:
		auth, authErr = atlassian.NewOAuthClientCredentialsAuthenticator(oauthClientID, oauthClientSecret)
	default:
		resp.Diagnostics.AddError("No authentication configured",
			"Configure either API token auth (username + api_token) or OAuth 2.0 "+
				"(oauth_client_id + oauth_client_secret, with optional oauth_refresh_token). "+
				"Values can be set via provider attributes or environment variables "+
				"(ATLASSIAN_USERNAME, ATLASSIAN_API_TOKEN, ATLASSIAN_OAUTH_CLIENT_ID, etc.)")
		return
	}

	if authErr != nil {
		resp.Diagnostics.AddError("Authentication configuration error", authErr.Error())
		return
	}

	// Create the API client
	apiClient, err := atlassian.NewClient(clientConfig, auth)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create Atlassian API client", err.Error())
		return
	}

	// Make the client available to resources and data sources
	resp.ResourceData = apiClient
	resp.DataSourceData = apiClient
}

// stringValueOrEnv returns the Terraform config value if set, otherwise the environment variable.
func stringValueOrEnv(val types.String, envVar string) string {
	if !val.IsNull() && !val.IsUnknown() {
		return val.ValueString()
	}
	return os.Getenv(envVar)
}

// Resources defines the resources implemented in the provider.
func (p *AtlassianProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		userrs.NewResource,
		groupresource.NewResource,
		groupresource.NewMembershipResource,
		roleresource.NewResource,
		roleresource.NewAssignmentResource,
		tokenresource.NewResource,
		spaceresource.NewResource,
		customdomainrs.NewResource,
		issuetyperesource.NewResource,
		issuetyperesource.NewSchemeResource,
		workflowresource.NewResource,
		workflowresource.NewSchemeResource,
		screenresource.NewResource,
		screenresource.NewSchemeResource,
		screenresource.NewTabFieldResource,
		permissionschemers.NewResource,
		securityschemers.NewResource,
		notificationschemers.NewResource,
		dashboardresource.NewResource,
		dashboardresource.NewFilterResource,
		customfieldresource.NewResource,
		boardresource.NewResource,
		priorityresource.NewResource,
		priorityresource.NewSchemeResource,
		automationresource.NewResource,
		mailhandlerresource.NewIncomingResource,
		mailhandlerresource.NewOutgoingResource,
		customdomainrs.NewEmailResource,
		confluencespaceresource.NewResource,
		confluencepageresource.NewResource,
		confluencepageresource.NewRestrictionResource,
		confluencetemplateresource.NewResource,
		confluencespacepermresource.NewPermissionResource,
		bbreporesource.NewResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *AtlassianProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		userds.NewDataSource,
		groupdatasource.NewDataSource,
		roledatasource.NewDataSource,
		spacedatasource.NewDataSource,
		customdomainds.NewDataSource,
		customdomainds.NewEmailDataSource,
		issuetypedatasource.NewDataSource,
		issuetypedatasource.NewSchemeDataSource,
		workflowdatasource.NewDataSource,
		workflowdatasource.NewSchemeDataSource,
		screendatasource.NewDataSource,
		screendatasource.NewSchemeDataSource,
		permissionschemeds.NewDataSource,
		securityschemeds.NewDataSource,
		notificationschemeds.NewDataSource,
		dashboarddatasource.NewDataSource,
		dashboarddatasource.NewFilterDataSource,
		customfielddatasource.NewDataSource,
		boarddatasource.NewDataSource,
		prioritydatasource.NewDataSource,
		prioritydatasource.NewSchemeDataSource,
		automationdatasource.NewDataSource,
		mailhandlerdatasource.NewIncomingDataSource,
		mailhandlerdatasource.NewOutgoingDataSource,
		confluencespacedatasource.NewDataSource,
		confluencepagedatasource.NewDataSource,
		confluencepagedatasource.NewRestrictionDataSource,
		confluencetemplatedatasource.NewDataSource,
		confluencespacepermdatasource.NewPermissionDataSource,
		bbrepodatasource.NewDataSource,
	}
}
