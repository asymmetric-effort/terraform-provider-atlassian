// Package unit contains unit tests for the Atlassian provider.
package unit

import (
	"context"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/provider"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testProtoV6ProviderFactories returns provider factories for testing.
func testProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"atlassian": providerserver.NewProtocol6WithError(provider.New("test")()),
	}
}

// TestProviderMetadata verifies the provider type name and version.
func TestProviderMetadata(t *testing.T) {
	t.Parallel()

	p := provider.New("1.0.0-test")()

	req := frameworkprovider.MetadataRequest{}
	resp := &frameworkprovider.MetadataResponse{}
	p.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian" {
		t.Errorf("expected provider type name 'atlassian', got %q", resp.TypeName)
	}
	if resp.Version != "1.0.0-test" {
		t.Errorf("expected version '1.0.0-test', got %q", resp.Version)
	}
}

// TestProviderSchema verifies the provider schema has all expected attributes.
func TestProviderSchema(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	req := frameworkprovider.SchemaRequest{}
	resp := &frameworkprovider.SchemaResponse{}
	p.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{
		"url", "username", "api_token",
		"oauth_client_id", "oauth_client_secret", "oauth_refresh_token",
		"request_timeout", "max_retries", "retry_wait_min", "retry_wait_max",
	}

	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestProviderSchemaAttributeCount verifies no unexpected attributes exist.
func TestProviderSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	req := frameworkprovider.SchemaRequest{}
	resp := &frameworkprovider.SchemaResponse{}
	p.Schema(context.Background(), req, resp)

	expected := 10
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestProviderSensitiveAttributes verifies sensitive fields are marked correctly.
func TestProviderSensitiveAttributes(t *testing.T) {
	t.Parallel()

	p := provider.New("test")()

	req := frameworkprovider.SchemaRequest{}
	resp := &frameworkprovider.SchemaResponse{}
	p.Schema(context.Background(), req, resp)

	sensitiveAttrs := []string{"api_token", "oauth_client_secret", "oauth_refresh_token"}
	for _, name := range sensitiveAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsSensitive() {
			t.Errorf("expected attribute %q to be marked sensitive", name)
		}
	}
}
