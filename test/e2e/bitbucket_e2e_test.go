// Package e2e runs end-to-end tests using real OpenTofu configurations
// against the mock Atlassian API server for Bitbucket resources.
package e2e

import (
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// bitbucketMockServer starts a mock with all endpoints for Bitbucket e2e tests.
func bitbucketMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterBitbucketEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestE2E_BitbucketRepository_CreateUpdateDestroy exercises the full tofu
// plan/apply/update/destroy lifecycle for atlassian_bitbucket_repository.
func TestE2E_BitbucketRepository_CreateUpdateDestroy(t *testing.T) {
	ts := bitbucketMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_bitbucket_repository" "test" {
  workspace   = "e2e-workspace"
  slug        = "e2e-repo"
  name        = "E2E Test Repo"
  description = "Created by e2e test"
  is_private  = true
  fork_policy = "no_forks"
  language    = "go"
  has_issues  = true
  has_wiki    = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_bitbucket_repository.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "workspace",
						"e2e-workspace"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "slug",
						"e2e-repo"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "name",
						"E2E Test Repo"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "description",
						"Created by e2e test"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "is_private",
						"true"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "fork_policy",
						"no_forks"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "language",
						"go"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "has_issues",
						"true"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "has_wiki",
						"false"),
					resource.TestCheckResourceAttrSet(
						"atlassian_bitbucket_repository.test", "url"),
					resource.TestCheckResourceAttrSet(
						"atlassian_bitbucket_repository.test", "clone_ssh"),
					resource.TestCheckResourceAttrSet(
						"atlassian_bitbucket_repository.test", "clone_https"),
				),
			},
			// Update name and description
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_bitbucket_repository" "test" {
  workspace   = "e2e-workspace"
  slug        = "e2e-repo"
  name        = "Updated E2E Repo"
  description = "Updated by e2e test"
  is_private  = true
  fork_policy = "no_forks"
  language    = "go"
  has_issues  = true
  has_wiki    = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "name",
						"Updated E2E Repo"),
					resource.TestCheckResourceAttr(
						"atlassian_bitbucket_repository.test", "description",
						"Updated by e2e test"),
				),
			},
			// Destroy is handled automatically by the test framework.
		},
	})
}

// TestE2E_BitbucketRepository_DataSource exercises the bitbucket repository
// data source by creating a repo then reading it via data source.
func TestE2E_BitbucketRepository_DataSource(t *testing.T) {
	ts := bitbucketMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_bitbucket_repository" "src" {
  workspace   = "ds-workspace"
  slug        = "ds-repo"
  name        = "DS Test Repo"
  description = "Data source e2e test"
  is_private  = false
  fork_policy = "allow_forks"
  has_issues  = false
  has_wiki    = true
}

data "atlassian_bitbucket_repository" "lookup" {
  workspace = atlassian_bitbucket_repository.src.workspace
  slug      = atlassian_bitbucket_repository.src.slug
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.atlassian_bitbucket_repository.lookup",
						"name", "DS Test Repo"),
					resource.TestCheckResourceAttr(
						"data.atlassian_bitbucket_repository.lookup",
						"description", "Data source e2e test"),
					resource.TestCheckResourceAttr(
						"data.atlassian_bitbucket_repository.lookup",
						"is_private", "false"),
					resource.TestCheckResourceAttr(
						"data.atlassian_bitbucket_repository.lookup",
						"fork_policy", "allow_forks"),
					resource.TestCheckResourceAttr(
						"data.atlassian_bitbucket_repository.lookup",
						"has_issues", "false"),
					resource.TestCheckResourceAttr(
						"data.atlassian_bitbucket_repository.lookup",
						"has_wiki", "true"),
					resource.TestCheckResourceAttrSet(
						"data.atlassian_bitbucket_repository.lookup",
						"id"),
					resource.TestCheckResourceAttrSet(
						"data.atlassian_bitbucket_repository.lookup",
						"url"),
				),
			},
		},
	})
}
