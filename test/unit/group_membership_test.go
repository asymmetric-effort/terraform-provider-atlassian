// Package unit contains unit tests for the Atlassian provider.
package unit

import (
	"context"
	"testing"

	groupresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/group"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestGroupMembershipResourceMetadata verifies the resource type name.
func TestGroupMembershipResourceMetadata(t *testing.T) {
	t.Parallel()

	r := groupresource.NewMembershipResource()

	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)

	if resp.TypeName != "atlassian_group_membership" {
		t.Errorf("expected resource type name 'atlassian_group_membership', got %q", resp.TypeName)
	}
}

// TestGroupMembershipResourceSchema verifies the resource schema has all expected attributes.
func TestGroupMembershipResourceSchema(t *testing.T) {
	t.Parallel()

	r := groupresource.NewMembershipResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	if resp.Schema.Attributes == nil {
		t.Fatal("expected schema to have attributes")
	}

	expectedAttrs := []string{"group_id", "user_account_ids"}
	for _, attr := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[attr]; !ok {
			t.Errorf("expected schema to have attribute %q", attr)
		}
	}
}

// TestGroupMembershipResourceSchemaAttributeCount verifies no unexpected attributes exist.
func TestGroupMembershipResourceSchemaAttributeCount(t *testing.T) {
	t.Parallel()

	r := groupresource.NewMembershipResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	expected := 2
	actual := len(resp.Schema.Attributes)
	if actual != expected {
		t.Errorf("expected %d schema attributes, got %d", expected, actual)
	}
}

// TestGroupMembershipResourceSchemaGroupIDRequired verifies the group_id attribute is required.
func TestGroupMembershipResourceSchemaGroupIDRequired(t *testing.T) {
	t.Parallel()

	r := groupresource.NewMembershipResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["group_id"]
	if !ok {
		t.Fatal("expected schema to have attribute 'group_id'")
	}
	if !attr.IsRequired() {
		t.Error("expected 'group_id' attribute to be required")
	}
}

// TestGroupMembershipResourceSchemaUserAccountIDsRequired verifies the user_account_ids attribute is required.
func TestGroupMembershipResourceSchemaUserAccountIDsRequired(t *testing.T) {
	t.Parallel()

	r := groupresource.NewMembershipResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["user_account_ids"]
	if !ok {
		t.Fatal("expected schema to have attribute 'user_account_ids'")
	}
	if !attr.IsRequired() {
		t.Error("expected 'user_account_ids' attribute to be required")
	}
}

// TestGroupMembershipResourceSchemaUserAccountIDsType verifies the user_account_ids attribute is a list of strings.
func TestGroupMembershipResourceSchemaUserAccountIDsType(t *testing.T) {
	t.Parallel()

	r := groupresource.NewMembershipResource()

	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), req, resp)

	attr, ok := resp.Schema.Attributes["user_account_ids"]
	if !ok {
		t.Fatal("expected schema to have attribute 'user_account_ids'")
	}

	expectedType := types.ListType{ElemType: types.StringType}
	if attr.GetType().String() != expectedType.String() {
		t.Errorf("expected 'user_account_ids' to be %s, got %s", expectedType, attr.GetType())
	}
}

// TestGroupMembershipResourceImplementsResource verifies the resource satisfies the Resource interface.
func TestGroupMembershipResourceImplementsResource(t *testing.T) {
	t.Parallel()

	var _ resource.Resource = groupresource.NewMembershipResource()
}

// TestGroupMembershipResourceImplementsImportState verifies the resource satisfies the ResourceWithImportState interface.
func TestGroupMembershipResourceImplementsImportState(t *testing.T) {
	t.Parallel()

	var _ resource.ResourceWithImportState = groupresource.NewMembershipResource().(resource.ResourceWithImportState)
}
