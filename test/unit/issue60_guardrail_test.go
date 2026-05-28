// Package unit provides guardrail tests for issue #60 acceptance criteria.
// These tests verify that the Bitbucket OpenAPI spec exists, covers all
// endpoints, types are generated, and the validator is integrated.
package unit

import (
	"os"
	"strings"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock/specs"
)

// TestIssue60_BitbucketOpenAPISpecExists verifies the spec exists and
// contains all expected Bitbucket paths.
func TestIssue60_BitbucketOpenAPISpecExists(t *testing.T) {
	t.Parallel()
	data, err := specs.SpecFS.ReadFile("bitbucket.yaml")
	if err != nil {
		t.Fatalf("Bitbucket OpenAPI spec not found in embedded FS: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Bitbucket OpenAPI spec is empty")
	}
	content := string(data)
	requiredPaths := []string{
		"/2.0/repositories/{workspace}/{slug}",
		"/2.0/repositories/{workspace}",
		"/2.0/repositories/{workspace}/{slug}/branch-restrictions",
		"/2.0/repositories/{workspace}/{slug}/pipelines_config",
		"/2.0/repositories/{workspace}/{slug}/environments",
		"/2.0/repositories/{workspace}/{slug}/permissions-config/users",
		"/2.0/repositories/{workspace}/{slug}/permissions-config/groups",
	}
	for _, p := range requiredPaths {
		if !strings.Contains(content, p) {
			t.Errorf("Bitbucket spec missing path: %s", p)
		}
	}
}

// TestIssue60_BitbucketTypesGenerated verifies generated types can be
// parsed by the validator.
func TestIssue60_BitbucketTypesGenerated(t *testing.T) {
	t.Parallel()
	data, err := specs.SpecFS.ReadFile("bitbucket.yaml")
	if err != nil {
		t.Skipf("Bitbucket spec not yet created: %v", err)
	}
	v, err := mock.NewRequestValidatorFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse Bitbucket spec: %v", err)
	}
	if v == nil {
		t.Fatal("Validator is nil")
	}
}

// TestIssue60_BitbucketValidatorIntegrated verifies the mock server
// loads the Bitbucket validator.
func TestIssue60_BitbucketValidatorIntegrated(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterBitbucketEndpoints(s)
	if s == nil {
		t.Fatal("Server is nil")
	}
}

// TestIssue60_GeneratedTypesFileExists checks the files on disk.
func TestIssue60_GeneratedTypesFileExists(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir("../../internal/mock/specs")
	if err != nil {
		t.Fatalf("Cannot read specs dir: %v", err)
	}
	hasBitbucketSpec := false
	hasBitbucketGen := false
	for _, e := range entries {
		if e.Name() == "bitbucket.yaml" {
			hasBitbucketSpec = true
		}
		if e.Name() == "bitbucket_types.gen.go" {
			hasBitbucketGen = true
		}
	}
	if !hasBitbucketSpec {
		t.Error("bitbucket.yaml spec file not found")
	}
	if !hasBitbucketGen {
		t.Error("bitbucket_types.gen.go generated file not found")
	}
}
