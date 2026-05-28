// Package unit contains unit tests for the Atlassian provider.
package unit

import (
	"context"
	"testing"

	userds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/user"
	userrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/user"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestUserResourceMetadata verifies the resource type name.
func TestUserResourceMetadata(t *testing.T) {
	t.Parallel()

	r := userrs.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_user" {
		t.Errorf("expected resource type name 'atlassian_user', got %q", resp.TypeName)
	}
}

// TestUserResourceSchema verifies the resource schema has all expected attributes.
func TestUserResourceSchema(t *testing.T) {
	t.Parallel()

	r := userrs.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"account_id", "email", "display_name", "active"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestUserResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestUserResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := userrs.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 5
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestUserResourceSchemaRequiredAttributes verifies required attributes.
func TestUserResourceSchemaRequiredAttributes(t *testing.T) {
	t.Parallel()

	r := userrs.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	requiredAttrs := []string{"email", "display_name"}
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

// TestUserResourceSchemaComputedAttributes verifies computed attributes.
func TestUserResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := userrs.NewResource()
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"account_id", "active"}
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

// TestUserResourceImplementsImportState verifies the resource implements ImportState.
func TestUserResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	r := userrs.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("expected user resource to implement ResourceWithImportState")
	}
}

// TestUserDataSourceMetadata verifies the data source type name.
func TestUserDataSourceMetadata(t *testing.T) {
	t.Parallel()

	ds := userds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_user" {
		t.Errorf("expected data source type name 'atlassian_user', got %q", resp.TypeName)
	}
}

// TestUserDataSourceSchema verifies the data source schema has all expected attributes.
func TestUserDataSourceSchema(t *testing.T) {
	t.Parallel()

	ds := userds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"account_id", "email", "display_name", "active"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestUserDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestUserDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	ds := userds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	expected := 5
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestUserDataSourceSchemaComputedAttributes verifies computed-only attributes.
func TestUserDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	ds := userds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	computedOnlyAttrs := []string{"display_name", "active"}
	for _, name := range computedOnlyAttrs {
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

// TestUserDataSourceSchemaOptionalAttributes verifies optional lookup attributes.
func TestUserDataSourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	ds := userds.NewDataSource()
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ds.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"account_id", "email"}
	for _, name := range optionalAttrs {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("expected attribute %q to exist", name)
			continue
		}
		if !attr.IsOptional() {
			t.Errorf("expected attribute %q to be optional", name)
		}
	}
}
