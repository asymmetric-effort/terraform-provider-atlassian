// Package unit contains tests for schema validators.
package unit

import (
	"context"
	"testing"

	roleresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/identity/role"
	fwpath "github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	fwschema "github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// getValidatorsFromSchema extracts validators from a specific attribute.
func getStringValidators(t *testing.T, s schema.Schema, attrName string) []fwschema.String {
	t.Helper()
	attr, ok := s.Attributes[attrName]
	if !ok {
		t.Fatalf("attribute %q not found", attrName)
	}
	sa, ok := attr.(schema.StringAttribute)
	if !ok {
		t.Fatalf("attribute %q is not StringAttribute", attrName)
	}
	return sa.Validators
}

// TestScopeValidatorValid tests valid scope values.
func TestScopeValidatorValid(t *testing.T) {
	t.Parallel()
	r := roleresource.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	validators := getStringValidators(t, resp.Schema, "scope")

	for _, v := range validators {
		// Test valid values
		for _, val := range []string{"org", "product"} {
			req := fwschema.StringRequest{
				ConfigValue: types.StringValue(val),
				Path:        fwpath.Root("scope"),
			}
			vResp := &fwschema.StringResponse{}
			v.ValidateString(context.Background(), req, vResp)
			if vResp.Diagnostics.HasError() {
				t.Errorf("scope %q should be valid, got error: %v", val, vResp.Diagnostics.Errors())
			}
		}

		// Test invalid value
		req := fwschema.StringRequest{
			ConfigValue: types.StringValue("invalid"),
			Path:        fwpath.Root("scope"),
		}
		vResp := &fwschema.StringResponse{}
		v.ValidateString(context.Background(), req, vResp)
		if !vResp.Diagnostics.HasError() {
			t.Error("scope 'invalid' should fail validation")
		}

		// Test null value (should not error)
		req2 := fwschema.StringRequest{
			ConfigValue: types.StringNull(),
			Path:        fwpath.Root("scope"),
		}
		vResp2 := &fwschema.StringResponse{}
		v.ValidateString(context.Background(), req2, vResp2)
		if vResp2.Diagnostics.HasError() {
			t.Error("null scope should not error")
		}

		// Test unknown value (should not error)
		req3 := fwschema.StringRequest{
			ConfigValue: types.StringUnknown(),
			Path:        fwpath.Root("scope"),
		}
		vResp3 := &fwschema.StringResponse{}
		v.ValidateString(context.Background(), req3, vResp3)
		if vResp3.Diagnostics.HasError() {
			t.Error("unknown scope should not error")
		}

		// Test Description and MarkdownDescription
		desc := v.Description(context.Background())
		if desc == "" {
			t.Error("expected non-empty Description")
		}
		mdDesc := v.MarkdownDescription(context.Background())
		if mdDesc == "" {
			t.Error("expected non-empty MarkdownDescription")
		}
	}
}

// TestPrincipalTypeValidatorValid tests valid principal_type values.
func TestPrincipalTypeValidatorValid(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	validators := getStringValidators(t, resp.Schema, "principal_type")

	for _, v := range validators {
		// Test valid values
		for _, val := range []string{"user", "group"} {
			req := fwschema.StringRequest{
				ConfigValue: types.StringValue(val),
				Path:        fwpath.Root("principal_type"),
			}
			vResp := &fwschema.StringResponse{}
			v.ValidateString(context.Background(), req, vResp)
			if vResp.Diagnostics.HasError() {
				t.Errorf("principal_type %q should be valid", val)
			}
		}

		// Test invalid value
		req := fwschema.StringRequest{
			ConfigValue: types.StringValue("invalid"),
			Path:        fwpath.Root("principal_type"),
		}
		vResp := &fwschema.StringResponse{}
		v.ValidateString(context.Background(), req, vResp)
		if !vResp.Diagnostics.HasError() {
			t.Error("principal_type 'invalid' should fail")
		}

		// Test null (should not error)
		req2 := fwschema.StringRequest{ConfigValue: types.StringNull(), Path: fwpath.Root("principal_type")}
		vResp2 := &fwschema.StringResponse{}
		v.ValidateString(context.Background(), req2, vResp2)
		if vResp2.Diagnostics.HasError() {
			t.Error("null should not error")
		}

		// Description
		desc := v.Description(context.Background())
		if desc == "" {
			t.Error("expected non-empty Description")
		}
		mdDesc := v.MarkdownDescription(context.Background())
		if mdDesc == "" {
			t.Error("expected non-empty MarkdownDescription")
		}
	}
}

// TestAssignmentScopeValidator tests the scope validator on role assignment.
func TestAssignmentScopeValidator(t *testing.T) {
	t.Parallel()
	r := roleresource.NewAssignmentResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	validators := getStringValidators(t, resp.Schema, "scope")

	for _, v := range validators {
		req := fwschema.StringRequest{
			ConfigValue: types.StringValue("invalid"),
			Path:        fwpath.Root("scope"),
		}
		vResp := &fwschema.StringResponse{}
		v.ValidateString(context.Background(), req, vResp)
		if !vResp.Diagnostics.HasError() {
			t.Error("Expected error for invalid scope")
		}
	}
}
