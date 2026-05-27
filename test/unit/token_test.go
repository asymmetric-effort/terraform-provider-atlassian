// Package unit contains unit tests for the Atlassian provider.
package unit

import (
	"context"
	"testing"

	tokenresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/token"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestTokenResourceMetadata verifies the resource type name.
func TestTokenResourceMetadata(t *testing.T) {
	t.Parallel()

	r := tokenresource.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_api_token" {
		t.Errorf("expected resource type name 'atlassian_api_token', got %q", resp.TypeName)
	}
}

// TestTokenResourceSchema verifies the resource schema has all expected attributes.
func TestTokenResourceSchema(t *testing.T) {
	t.Parallel()

	r := tokenresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"token_id", "label", "user_account_id", "token_value", "created_at"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestTokenResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestTokenResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := tokenresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 5
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestTokenResourceSchemaRequiredAttributes verifies required attributes.
func TestTokenResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := tokenresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"label", "user_account_id"}
	for _, name := range requiredAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsRequired() {
			t.Errorf("expected attribute %q to be required", name)
		}
	}
}

// TestTokenResourceSchemaComputedAttributes verifies computed attributes.
func TestTokenResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := tokenresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"token_id", "token_value", "created_at"}
	for _, name := range computedAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestTokenResourceSchemaSensitiveAttributes verifies the token_value attribute is marked sensitive.
func TestTokenResourceSchemaSensitiveAttributes(t *testing.T) {
	t.Parallel()

	r := tokenresource.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["token_value"]
	if !ok {
		t.Fatal("expected schema to have attribute 'token_value'")
	}
	if !attr.IsSensitive() {
		t.Error("expected attribute 'token_value' to be marked sensitive")
	}
}

// TestTokenResourceImplementsResource verifies the resource satisfies the Resource interface.
func TestTokenResourceImplementsResource(t *testing.T) {
	t.Parallel()

	var _ resource.Resource = tokenresource.NewResource()
}

// TestTokenResourceImplementsImportState verifies the resource satisfies the ResourceWithImportState interface.
func TestTokenResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := tokenresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected token resource to implement ResourceWithImportState")
	}
}
