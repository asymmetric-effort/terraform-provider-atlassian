// Package e2e runs end-to-end tests using real OpenTofu configurations
// against the mock Atlassian API server for Jira resources.
package e2e

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// jiraMockServer starts a mock with all endpoints for Jira e2e tests.
func jiraMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterJiraEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestE2E_JiraSpace_CreateUpdateDestroy exercises the full tofu
// plan/apply/update/destroy cycle for atlassian_jira_space.
func TestE2E_JiraSpace_CreateUpdateDestroy(t *testing.T) {
	ts := jiraMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_jira_space" "test" {
  key        = "E2ETEST"
  name       = "E2E Test Space"
  space_type = "classic"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_jira_space.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_jira_space.test", "key",
						"E2ETEST"),
					resource.TestCheckResourceAttr(
						"atlassian_jira_space.test", "name",
						"E2E Test Space"),
					resource.TestCheckResourceAttrSet(
						"atlassian_jira_space.test", "self_url"),
					resource.TestCheckResourceAttrSet(
						"atlassian_jira_space.test", "browse_url"),
				),
			},
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_jira_space" "test" {
  key        = "E2ETEST"
  name       = "Updated E2E Space"
  space_type = "classic"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"atlassian_jira_space.test", "name",
						"Updated E2E Space"),
				),
			},
		},
	})
}

// TestE2E_JiraWorkflow_CreateDestroy exercises workflow lifecycle.
func TestE2E_JiraWorkflow_CreateDestroy(t *testing.T) {
	ts := jiraMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_jira_workflow" "test" {
  name        = "E2E Workflow"
  description = "Created by e2e test"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_jira_workflow.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_jira_workflow.test", "name",
						"E2E Workflow"),
				),
			},
		},
	})
}

// TestE2E_JiraDashboard_CreateDestroy exercises dashboard lifecycle.
func TestE2E_JiraDashboard_CreateDestroy(t *testing.T) {
	ts := jiraMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_jira_dashboard" "test" {
  name = "E2E Dashboard"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_jira_dashboard.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_jira_dashboard.test", "name",
						"E2E Dashboard"),
				),
			},
		},
	})
}

// TestE2E_JiraSpaceDataSource exercises data source with space.
func TestE2E_JiraSpaceDataSource(t *testing.T) {
	ts := jiraMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + fmt.Sprintf(`
resource "atlassian_jira_space" "src" {
  key        = "DSTEST"
  name       = "DS Test Space"
  space_type = "classic"
}

data "atlassian_jira_space" "lookup" {
  id = atlassian_jira_space.src.id
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.atlassian_jira_space.lookup",
						"key", "DSTEST"),
					resource.TestCheckResourceAttr(
						"data.atlassian_jira_space.lookup",
						"name", "DS Test Space"),
				),
			},
		},
	})
}
