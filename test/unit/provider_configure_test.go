// Package unit contains unit tests for provider Configure method.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/provider"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// getProviderSchema returns the provider schema for building test configs.
func getProviderSchema(t *testing.T) *frameworkprovider.SchemaResponse {
	t.Helper()
	p := provider.New("test")()
	resp := &frameworkprovider.SchemaResponse{}
	p.Schema(context.Background(), frameworkprovider.SchemaRequest{}, resp)
	return resp
}

// buildProviderConfig builds a tfsdk.Config from provider attribute values.
// Pass empty string for null/unset values.
func buildProviderConfig(t *testing.T, attrs map[string]interface{}) tfsdk.Config {
	t.Helper()
	schemaResp := getProviderSchema(t)
	ctx := context.Background()
	tfType := schemaResp.Schema.Type().TerraformType(ctx)

	values := map[string]tftypes.Value{
		"url":                 tftypes.NewValue(tftypes.String, nil),
		"username":            tftypes.NewValue(tftypes.String, nil),
		"api_token":           tftypes.NewValue(tftypes.String, nil),
		"oauth_client_id":     tftypes.NewValue(tftypes.String, nil),
		"oauth_client_secret": tftypes.NewValue(tftypes.String, nil),
		"oauth_refresh_token": tftypes.NewValue(tftypes.String, nil),
		"request_timeout":     tftypes.NewValue(tftypes.String, nil),
		"max_retries":         tftypes.NewValue(tftypes.Number, nil),
		"retry_wait_min":      tftypes.NewValue(tftypes.String, nil),
		"retry_wait_max":      tftypes.NewValue(tftypes.String, nil),
	}

	for k, v := range attrs {
		switch val := v.(type) {
		case string:
			values[k] = tftypes.NewValue(tftypes.String, val)
		case int64:
			values[k] = tftypes.NewValue(tftypes.Number, val)
		case int:
			values[k] = tftypes.NewValue(tftypes.Number, val)
		}
	}

	return tfsdk.Config{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(tfType, values),
	}
}

// TestProviderConfigureAPIToken tests provider Configure with API token auth.
func TestProviderConfigureAPIToken(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":       ts.URL,
		"username":  "test@example.com",
		"api_token": "test-token",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with API token failed: %v", resp.Diagnostics.Errors())
	}
	if resp.ResourceData == nil {
		t.Fatal("Expected non-nil ResourceData")
	}
	if resp.DataSourceData == nil {
		t.Fatal("Expected non-nil DataSourceData")
	}
}

// TestProviderConfigureOAuthClientCredentials tests Configure with OAuth client credentials.
func TestProviderConfigureOAuthClientCredentials(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":                 ts.URL,
		"oauth_client_id":     "client-id",
		"oauth_client_secret": "client-secret",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with OAuth client credentials failed: %v", resp.Diagnostics.Errors())
	}
	if resp.ResourceData == nil {
		t.Fatal("Expected non-nil ResourceData")
	}
}

// TestProviderConfigureOAuthRefreshToken tests Configure with OAuth refresh token.
func TestProviderConfigureOAuthRefreshToken(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":                 ts.URL,
		"oauth_client_id":     "client-id",
		"oauth_client_secret": "client-secret",
		"oauth_refresh_token": "refresh-token",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with OAuth refresh token failed: %v", resp.Diagnostics.Errors())
	}
}

// TestProviderConfigureNoAuth tests Configure with no authentication configured.
func TestProviderConfigureNoAuth(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url": "https://example.atlassian.net",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when no auth is configured")
	}
}

// TestProviderConfigureMissingURL tests Configure with missing URL.
func TestProviderConfigureMissingURL(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"username":  "test@example.com",
		"api_token": "test-token",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when URL is missing")
	}
}

// TestProviderConfigureInvalidRequestTimeout tests Configure with invalid request_timeout.
func TestProviderConfigureInvalidRequestTimeout(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":             "https://example.atlassian.net",
		"username":        "test@example.com",
		"api_token":       "test-token",
		"request_timeout": "not-a-duration",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid request_timeout")
	}
}

// TestProviderConfigureInvalidRetryWaitMin tests Configure with invalid retry_wait_min.
func TestProviderConfigureInvalidRetryWaitMin(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":            "https://example.atlassian.net",
		"username":       "test@example.com",
		"api_token":      "test-token",
		"retry_wait_min": "not-a-duration",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid retry_wait_min")
	}
}

// TestProviderConfigureInvalidRetryWaitMax tests Configure with invalid retry_wait_max.
func TestProviderConfigureInvalidRetryWaitMax(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":            "https://example.atlassian.net",
		"username":       "test@example.com",
		"api_token":      "test-token",
		"retry_wait_max": "not-a-duration",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid retry_wait_max")
	}
}

// TestProviderConfigureWithRetryParams tests Configure with valid retry parameters.
func TestProviderConfigureWithRetryParams(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":             ts.URL,
		"username":        "test@example.com",
		"api_token":       "test-token",
		"request_timeout": "10s",
		"max_retries":     int64(3),
		"retry_wait_min":  "500ms",
		"retry_wait_max":  "5s",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with retry params failed: %v", resp.Diagnostics.Errors())
	}
}

// TestProviderConfigureEnvVarFallback tests Configure with environment variable fallbacks.
func TestProviderConfigureEnvVarFallback(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer ts.Close()

	t.Setenv("ATLASSIAN_URL", ts.URL)
	t.Setenv("ATLASSIAN_USERNAME", "env@example.com")
	t.Setenv("ATLASSIAN_API_TOKEN", "env-token")

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Configure with env vars failed: %v", resp.Diagnostics.Errors())
	}
}

// TestProviderConfigureMissingUsernameForToken tests API token auth with missing username.
func TestProviderConfigureMissingUsernameForToken(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":       "https://example.atlassian.net",
		"api_token": "test-token",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error when username is missing for API token auth")
	}
}

// TestProviderConfigureInvalidURL tests Configure with an invalid URL.
func TestProviderConfigureInvalidURL(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()
	config := buildProviderConfig(t, map[string]interface{}{
		"url":       "not-a-valid-url",
		"username":  "test@example.com",
		"api_token": "test-token",
	})

	req := frameworkprovider.ConfigureRequest{Config: config}
	resp := &frameworkprovider.ConfigureResponse{}
	p.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for invalid URL")
	}
}

// TestProviderResources tests that provider returns the expected resources.
func TestProviderResources(t *testing.T) {
	t.Parallel()
	p := provider.New("test")()
	resources := p.Resources(context.Background())
	if len(resources) != 21 {
		t.Errorf("expected 21 resources, got %d", len(resources))
	}
}

// TestProviderDataSources tests that provider returns the expected data sources.
func TestProviderDataSources(t *testing.T) {
	t.Parallel()
	p := provider.New("test")()
	dataSources := p.DataSources(context.Background())
	if len(dataSources) != 17 {
		t.Errorf("expected 17 data sources, got %d", len(dataSources))
	}
}
