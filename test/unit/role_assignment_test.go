// Package unit contains unit tests for the Atlassian provider.
package unit

import (
	"context"
	"testing"

	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestRoleAssignmentResourceMetadata verifies the resource type name.
func TestRoleAssignmentResourceMetadata(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_role_assignment" {
		t.Errorf("expected resource type name 'atlassian_role_assignment', got %q", resp.TypeName)
	}
}

// TestRoleAssignmentResourceSchema verifies the resource schema has all expected attributes.
func TestRoleAssignmentResourceSchema(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"id", "role_id", "principal_type", "principal_id", "scope", "product_id"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestRoleAssignmentResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestRoleAssignmentResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 6
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestRoleAssignmentResourceSchemaRoleIDRequired verifies the role_id attribute is required.
func TestRoleAssignmentResourceSchemaRoleIDRequired(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["role_id"]
	if !ok {
		t.Fatal("expected schema to have attribute 'role_id'")
	}
	if !attr.IsRequired() {
		t.Error("expected 'role_id' attribute to be required")
	}
}

// TestRoleAssignmentResourceSchemaPrincipalTypeRequired verifies the principal_type attribute is required.
func TestRoleAssignmentResourceSchemaPrincipalTypeRequired(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["principal_type"]
	if !ok {
		t.Fatal("expected schema to have attribute 'principal_type'")
	}
	if !attr.IsRequired() {
		t.Error("expected 'principal_type' attribute to be required")
	}
}

// TestRoleAssignmentResourceSchemaPrincipalIDRequired verifies the principal_id attribute is required.
func TestRoleAssignmentResourceSchemaPrincipalIDRequired(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["principal_id"]
	if !ok {
		t.Fatal("expected schema to have attribute 'principal_id'")
	}
	if !attr.IsRequired() {
		t.Error("expected 'principal_id' attribute to be required")
	}
}

// TestRoleAssignmentResourceSchemaScopeRequired verifies the scope attribute is required.
func TestRoleAssignmentResourceSchemaScopeRequired(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["scope"]
	if !ok {
		t.Fatal("expected schema to have attribute 'scope'")
	}
	if !attr.IsRequired() {
		t.Error("expected 'scope' attribute to be required")
	}
}

// TestRoleAssignmentResourceSchemaProductIDOptional verifies the product_id attribute is optional.
func TestRoleAssignmentResourceSchemaProductIDOptional(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["product_id"]
	if !ok {
		t.Fatal("expected schema to have attribute 'product_id'")
	}
	if !attr.IsOptional() {
		t.Error("expected 'product_id' attribute to be optional")
	}
}

// TestRoleAssignmentResourceSchemaIDComputed verifies the id attribute is computed.
func TestRoleAssignmentResourceSchemaIDComputed(t *testing.T) {
	t.Parallel()

	r := roleresource.NewAssignmentResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("expected schema to have attribute 'id'")
	}
	if !attr.IsComputed() {
		t.Error("expected 'id' attribute to be computed")
	}
}

// TestRoleAssignmentResourceImplementsResource verifies the resource satisfies the Resource interface.
func TestRoleAssignmentResourceImplementsResource(t *testing.T) {
	t.Parallel()

	var _ resource.Resource = roleresource.NewAssignmentResource()
}

// TestRoleAssignmentResourceImplementsImportState verifies the resource satisfies the ResourceWithImportState interface.
func TestRoleAssignmentResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	var _ resource.ResourceWithImportState = roleresource.NewAssignmentResource().(resource.ResourceWithImportState)
}
