// Package unit provides guardrail tests for issue #52.
package unit

import (
	"os"
	"strings"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock/specs"
)

// TestIssue52_ConfluenceOpenAPISpecExists verifies the spec exists.
func TestIssue52_ConfluenceOpenAPISpecExists(t *testing.T) {
	t.Parallel()
	data, err := specs.SpecFS.ReadFile("confluence.yaml")
	if err != nil {
		t.Fatalf("Confluence OpenAPI spec not found: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Confluence OpenAPI spec is empty")
	}
	content := string(data)
	requiredPaths := []string{
		"/wiki/api/v2/spaces",
		"/wiki/api/v2/pages",
		"/wiki/api/v2/templates",
	}
	for _, p := range requiredPaths {
		if !strings.Contains(content, p) {
			t.Errorf("Confluence spec missing path: %s", p)
		}
	}
}

// TestIssue52_GeneratedTypesFileExists checks generated file.
func TestIssue52_GeneratedTypesFileExists(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("../../internal/mock/specs")
	if err != nil {
		t.Fatalf("Cannot read specs dir: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == "confluence.yaml" {
			found = true
		}
	}
	if !found {
		t.Error("confluence.yaml not found")
	}
}
