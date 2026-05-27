// Package unit contains unit tests for the Atlassian provider.
package unit

import (
	"context"
	"testing"

	roledatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/role"
	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestRoleResourceMetadata verifies the resource type name.
func TestRoleResourceMetadata(t *testing.T) {
	t.Parallel()

	r := roleresource.NewResource()

	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_role" {
		t.Errorf("expected resource type name 'atlassian_role', got %q", resp.TypeName)
	}
}

// TestRoleResourceSchema verifies the resource schema has all expected attributes.
func TestRoleResourceSchema(t *testing.T) {
	t.Parallel()

	r := roleresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"role_id", "name", "description", "scope"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestRoleResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestRoleResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := roleresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestRoleResourceSchemaNameRequired verifies the name attribute is required.
func TestRoleResourceSchemaNameRequired(t *testing.T) {
	t.Parallel()

	r := roleresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["name"]
	if !ok {
		t.Fatal("expected schema to have attribute 'name'")
	}
	if !attr.IsRequired() {
		t.Error("expected 'name' attribute to be required")
	}
}

// TestRoleResourceSchemaScopeRequired verifies the scope attribute is required.
func TestRoleResourceSchemaScopeRequired(t *testing.T) {
	t.Parallel()

	r := roleresource.NewResource()

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

// TestRoleResourceSchemaDescriptionOptional verifies the description attribute is optional.
func TestRoleResourceSchemaDescriptionOptional(t *testing.T) {
	t.Parallel()

	r := roleresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["description"]
	if !ok {
		t.Fatal("expected schema to have attribute 'description'")
	}
	if !attr.IsOptional() {
		t.Error("expected 'description' attribute to be optional")
	}
}

// TestRoleResourceSchemaComputedAttributes verifies computed attributes.
func TestRoleResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := roleresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"role_id", "description"}
	for _, name := range computedAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected schema to have attribute %q", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestRoleDataSourceMetadata verifies the data source type name.
func TestRoleDataSourceMetadata(t *testing.T) {
	t.Parallel()

	d := roledatasource.NewDataSource()

	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_role" {
		t.Errorf("expected data source type name 'atlassian_role', got %q", resp.TypeName)
	}
}

// TestRoleDataSourceSchema verifies the data source schema has all expected attributes.
func TestRoleDataSourceSchema(t *testing.T) {
	t.Parallel()

	d := roledatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"role_id", "name", "description", "scope"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestRoleDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestRoleDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	d := roledatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	expected := 4
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestRoleDataSourceSchemaOptionalAttributes verifies role_id and name are optional.
func TestRoleDataSourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	d := roledatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"role_id", "name"}
	for _, name := range optionalAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected schema to have attribute %q", name)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("expected attribute %q to be optional", name)
		}
	}
}

// TestRoleDataSourceSchemaComputedAttributes verifies computed attributes.
func TestRoleDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	d := roledatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	computedAttrs := []string{"role_id", "name", "description", "scope"}
	for _, name := range computedAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected schema to have attribute %q", name)
			continue
		}
		if !attr.IsComputed() {
			t.Errorf("expected attribute %q to be computed", name)
		}
	}
}

// TestRoleResourceImplementsResource verifies the resource satisfies the Resource interface.
func TestRoleResourceImplementsResource(t *testing.T) {
	t.Parallel()

	var _ resource.Resource = roleresource.NewResource()
}

// TestRoleResourceImplementsImportState verifies the resource satisfies the ResourceWithImportState interface.
func TestRoleResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	var _ resource.ResourceWithImportState = roleresource.NewResource().(resource.ResourceWithImportState)
}

// TestRoleDataSourceImplementsDataSource verifies the data source satisfies the DataSource interface.
func TestRoleDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	var _ datasource.DataSource = roledatasource.NewDataSource()
}
