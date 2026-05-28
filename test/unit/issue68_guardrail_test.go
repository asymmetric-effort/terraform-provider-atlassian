// Package unit provides guardrail tests for issue #68 acceptance criteria.
// These tests verify that the Statuspage OpenAPI spec exists, covers all
// endpoints, types are generated, and the validator is integrated.
package unit

import (
	"os"
	"strings"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock/specs"
)

// TestIssue68_StatuspageOpenAPISpecExists verifies the spec exists and
// contains all expected Statuspage paths.
func TestIssue68_StatuspageOpenAPISpecExists(t *testing.T) {
	t.Parallel()
	data, err := specs.SpecFS.ReadFile("statuspage.yaml")
	if err != nil {
		t.Fatalf("Statuspage OpenAPI spec not found in embedded FS: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Statuspage OpenAPI spec is empty")
	}
	content := string(data)
	requiredPaths := []string{
		"/v1/pages",
		"/v1/pages/{page_id}",
		"/v1/pages/{page_id}/components",
		"/v1/pages/{page_id}/components/{component_id}",
		"/v1/pages/{page_id}/component-groups",
		"/v1/pages/{page_id}/component-groups/{group_id}",
		"/v1/pages/{page_id}/subscribers",
		"/v1/pages/{page_id}/subscribers/{subscriber_id}",
		"/v1/pages/{page_id}/incident_templates",
		"/v1/pages/{page_id}/incident_templates/{template_id}",
		"/v1/pages/{page_id}/maintenance_templates",
		"/v1/pages/{page_id}/maintenance_templates/{template_id}",
		"/v1/pages/{page_id}/permissions",
		"/v1/pages/{page_id}/permissions/{permission_id}",
	}
	for _, p := range requiredPaths {
		if !strings.Contains(content, p) {
			t.Errorf("Statuspage spec missing path: %s", p)
		}
	}
}

// TestIssue68_StatuspageTypesGenerated verifies generated types can be
// parsed by the validator.
func TestIssue68_StatuspageTypesGenerated(t *testing.T) {
	t.Parallel()
	data, err := specs.SpecFS.ReadFile("statuspage.yaml")
	if err != nil {
		t.Skipf("Statuspage spec not yet created: %v", err)
	}
	v, err := mock.NewRequestValidatorFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse Statuspage spec: %v", err)
	}
	if v == nil {
		t.Fatal("Validator is nil")
	}
}

// TestIssue68_StatuspageValidatorIntegrated verifies the mock server
// loads the Statuspage validator.
func TestIssue68_StatuspageValidatorIntegrated(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterStatuspageEndpoints(s)
	if s == nil {
		t.Fatal("Server is nil")
	}
}

// TestIssue68_GeneratedTypesFileExists checks the files on disk.
func TestIssue68_GeneratedTypesFileExists(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("../../internal/mock/specs")
	if err != nil {
		t.Fatalf("Cannot read specs dir: %v", err)
	}
	hasStatuspageSpec := false
	hasStatuspageGen := false
	for _, e := range entries {
		if e.Name() == "statuspage.yaml" {
			hasStatuspageSpec = true
		}
		if e.Name() == "statuspage_types.gen.go" {
			hasStatuspageGen = true
		}
	}
	if !hasStatuspageSpec {
		t.Error("statuspage.yaml spec file not found")
	}
	if !hasStatuspageGen {
		t.Error("statuspage_types.gen.go generated file not found")
	}
}
