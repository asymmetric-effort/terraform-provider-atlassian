// Package e2e runs end-to-end tests for Confluence resources.
package e2e

import (
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// confluenceMockServer starts a mock with Confluence endpoints.
func confluenceMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterConfluenceEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestE2E_ConfluenceSpace_CreateUpdateDestroy exercises the full
// tofu plan/apply/update/destroy cycle for atlassian_confluence_space.
func TestE2E_ConfluenceSpace_CreateUpdateDestroy(t *testing.T) {
	ts := confluenceMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_confluence_space" "test" {
  key  = "E2ECNFL"
  name = "E2E Confluence Space"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_confluence_space.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_confluence_space.test", "key",
						"E2ECNFL"),
					resource.TestCheckResourceAttr(
						"atlassian_confluence_space.test", "name",
						"E2E Confluence Space"),
				),
			},
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_confluence_space" "test" {
  key  = "E2ECNFL"
  name = "Updated Confluence Space"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"atlassian_confluence_space.test", "name",
						"Updated Confluence Space"),
				),
			},
		},
	})
}

// TestE2E_ConfluenceTemplate_CreateDestroy exercises template lifecycle.
func TestE2E_ConfluenceTemplate_CreateDestroy(t *testing.T) {
	ts := confluenceMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_confluence_template" "test" {
  name        = "E2E Template"
  description = "Created by e2e test"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_confluence_template.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_confluence_template.test",
						"name", "E2E Template"),
				),
			},
		},
	})
}
