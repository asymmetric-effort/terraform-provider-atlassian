// Package e2e runs end-to-end tests for Statuspage resources.
package e2e

import (
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// statuspageMockServer starts a mock with Statuspage endpoints.
func statuspageMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterStatuspageEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// TestE2E_StatuspagePage_CreateUpdateDestroy exercises the full
// tofu plan/apply/update/destroy cycle for atlassian_statuspage_page.
func TestE2E_StatuspagePage_CreateUpdateDestroy(t *testing.T) {
	ts := statuspageMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_statuspage_page" "test" {
  name             = "E2E Status Page"
  page_description = "Created by e2e test"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_statuspage_page.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_statuspage_page.test", "name",
						"E2E Status Page"),
				),
			},
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_statuspage_page" "test" {
  name             = "Updated Status Page"
  page_description = "Updated by e2e"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"atlassian_statuspage_page.test", "name",
						"Updated Status Page"),
				),
			},
		},
	})
}

// TestE2E_StatuspageComponent_CreateDestroy exercises component lifecycle.
func TestE2E_StatuspageComponent_CreateDestroy(t *testing.T) {
	ts := statuspageMockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_statuspage_page" "pg" {
  name = "Component Test Page"
}

resource "atlassian_statuspage_component" "test" {
  page_id = atlassian_statuspage_page.pg.id
  name    = "E2E Component"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_statuspage_component.test", "id"),
					resource.TestCheckResourceAttr(
						"atlassian_statuspage_component.test",
						"name", "E2E Component"),
				),
			},
		},
	})
}
