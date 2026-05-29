// Package e2e runs end-to-end tests using real OpenTofu configurations
// against the mock Atlassian API server. These tests exercise the full
// tofu plan/apply/destroy cycle through the provider framework.
package e2e

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// mockServer starts a mock Atlassian API server for e2e tests.
func mockServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// providerFactories returns the provider factories for e2e tests.
func providerFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"atlassian": providerserver.NewProtocol6WithError(
			provider.New("e2e-test")(),
		),
	}
}

// providerConfig returns the provider HCL block pointing at the mock.
func providerConfig(mockURL string) string {
	return fmt.Sprintf(`
provider "atlassian" {
  url     = %q
  api_key = "test-api-key"
}
`, mockURL)
}

// TestE2E_UserResource_CreateReadUpdateDestroy exercises the full
// tofu plan → apply → update → destroy lifecycle for atlassian_user.
func TestE2E_UserResource_CreateReadUpdateDestroy(t *testing.T) {
	ts := mockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			// Create and Read
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_user" "test" {
  email        = "e2e-user@example.com"
  display_name = "E2E Test User"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_user.test", "account_id"),
					resource.TestCheckResourceAttr(
						"atlassian_user.test", "email",
						"e2e-user@example.com"),
					resource.TestCheckResourceAttr(
						"atlassian_user.test", "display_name",
						"E2E Test User"),
					resource.TestCheckResourceAttr(
						"atlassian_user.test", "active", "true"),
					resource.TestCheckResourceAttrSet(
						"atlassian_user.test", "self_url"),
				),
			},
			// Update display_name
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_user" "test" {
  email        = "e2e-user@example.com"
  display_name = "Updated E2E User"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"atlassian_user.test", "display_name",
						"Updated E2E User"),
				),
			},
			// Destroy is handled automatically by the test framework.
		},
	})
}

// TestE2E_GroupResource_CreateDestroy exercises the group lifecycle.
func TestE2E_GroupResource_CreateDestroy(t *testing.T) {
	ts := mockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_group" "test" {
  name = "e2e-test-group"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_group.test", "group_id"),
					resource.TestCheckResourceAttr(
						"atlassian_group.test", "name",
						"e2e-test-group"),
					resource.TestCheckResourceAttrSet(
						"atlassian_group.test", "self_url"),
				),
			},
		},
	})
}

// TestE2E_RoleResource_CreateDestroy exercises the role lifecycle.
func TestE2E_RoleResource_CreateDestroy(t *testing.T) {
	ts := mockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_role" "test" {
  name        = "e2e-test-role"
  description = "Role created by e2e test"
  scope       = "org"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet(
						"atlassian_role.test", "role_id"),
					resource.TestCheckResourceAttr(
						"atlassian_role.test", "name",
						"e2e-test-role"),
					resource.TestCheckResourceAttr(
						"atlassian_role.test", "scope", "org"),
				),
			},
		},
	})
}

// TestE2E_UserDataSource exercises the user data source.
func TestE2E_UserDataSource(t *testing.T) {
	ts := mockServer(t)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providerFactories(),
		Steps: []resource.TestStep{
			{
				Config: providerConfig(ts.URL) + `
resource "atlassian_user" "source" {
  email        = "ds-source@example.com"
  display_name = "DS Source User"
}

data "atlassian_user" "lookup" {
  account_id = atlassian_user.source.account_id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.atlassian_user.lookup",
						"display_name", "DS Source User"),
					resource.TestCheckResourceAttr(
						"data.atlassian_user.lookup",
						"active", "true"),
				),
			},
		},
	})
}

// Ensure context import is used.
var _ = context.Background
