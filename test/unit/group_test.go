// Package unit contains unit tests for the Atlassian provider.
package unit

import (
	"context"
	"testing"

	groupdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/identity/group"
	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestGroupResourceMetadata verifies the resource type name.
func TestGroupResourceMetadata(t *testing.T) {
	t.Parallel()

	r := groupresource.NewResource()

	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_group" {
		t.Errorf("expected resource type name 'atlassian_group', got %q", resp.TypeName)
	}
}

// TestGroupResourceSchema verifies the resource schema has all expected attributes.
func TestGroupResourceSchema(t *testing.T) {
	t.Parallel()

	r := groupresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"group_id", "name", "self_url"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestGroupResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestGroupResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := groupresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestGroupResourceSchemaNameRequired verifies the name attribute is required.
func TestGroupResourceSchemaNameRequired(t *testing.T) {
	t.Parallel()

	r := groupresource.NewResource()

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

// TestGroupResourceSchemaComputedAttributes verifies computed attributes.
func TestGroupResourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	r := groupresource.NewResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	computedAttrs := []string{"group_id", "self_url"}
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

// TestGroupDataSourceMetadata verifies the data source type name.
func TestGroupDataSourceMetadata(t *testing.T) {
	t.Parallel()

	d := groupdatasource.NewDataSource()

	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_group" {
		t.Errorf("expected data source type name 'atlassian_group', got %q", resp.TypeName)
	}
}

// TestGroupDataSourceSchema verifies the data source schema has all expected attributes.
func TestGroupDataSourceSchema(t *testing.T) {
	t.Parallel()

	d := groupdatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"group_id", "name", "self_url"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestGroupDataSourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestGroupDataSourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	d := groupdatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	expected := 3
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestGroupDataSourceSchemaOptionalAttributes verifies group_id and name are optional.
func TestGroupDataSourceSchemaOptionalAttributes(t *testing.T) {
	t.Parallel()

	d := groupdatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	optionalAttrs := []string{"group_id", "name"}
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

// TestGroupDataSourceSchemaComputedAttributes verifies computed attributes.
func TestGroupDataSourceSchemaComputedAttributes(t *testing.T) {
	t.Parallel()

	d := groupdatasource.NewDataSource()

	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), req, resp)

	computedAttrs := []string{"group_id", "name", "self_url"}
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

// TestGroupResourceImplementsResource verifies the resource satisfies the Resource interface.
func TestGroupResourceImplementsResource(t *testing.T) {
	t.Parallel()

	var _ resource.Resource = groupresource.NewResource()
}

// TestGroupResourceImplementsImportState verifies the resource satisfies the ResourceWithImportState interface.
func TestGroupResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	var _ resource.ResourceWithImportState = groupresource.NewResource().(resource.ResourceWithImportState)
}

// TestGroupDataSourceImplementsDataSource verifies the data source satisfies the DataSource interface.
func TestGroupDataSourceImplementsDataSource(t *testing.T) {
	t.Parallel()

	var _ datasource.DataSource = groupdatasource.NewDataSource()
}
