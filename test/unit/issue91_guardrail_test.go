// Package unit provides guardrail tests for issue #91.
package unit

import (
	"context"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/provider"
	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestIssue91_IssueTypeScreenSchemeResourceExists verifies the
// resource is registered in the provider.
func TestIssue91_IssueTypeScreenSchemeResourceExists(t *testing.T) {
	t.Parallel()
	p := provider.New("test")()
	resources := p.Resources(context.Background())
	found := false
	for _, rf := range resources {
		r := rf()
		resp := &fwresource.MetadataResponse{}
		r.Metadata(context.Background(), fwresource.MetadataRequest{
			ProviderTypeName: "atlassian",
		}, resp)
		if resp.TypeName == "atlassian_jira_issue_type_screen_scheme" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("atlassian_jira_issue_type_screen_scheme not registered")
	}
}

// TestIssue91_IssueTypeScreenSchemeDataSourceExists verifies the
// data source is registered.
func TestIssue91_IssueTypeScreenSchemeDataSourceExists(t *testing.T) {
	t.Parallel()
	p := provider.New("test")()
	dataSources := p.DataSources(context.Background())
	found := false
	for _, dsf := range dataSources {
		ds := dsf()
		resp := &fwdatasource.MetadataResponse{}
		ds.Metadata(context.Background(), fwdatasource.MetadataRequest{
			ProviderTypeName: "atlassian",
		}, resp)
		if resp.TypeName == "atlassian_jira_issue_type_screen_scheme" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("atlassian_jira_issue_type_screen_scheme data source not registered")
	}
}

// TestIssue91_ResourceHasImportState verifies ImportState is implemented.
func TestIssue91_ResourceHasImportState(t *testing.T) {
	t.Parallel()
	p := provider.New("test")()
	resources := p.Resources(context.Background())
	for _, rf := range resources {
		r := rf()
		resp := &fwresource.MetadataResponse{}
		r.Metadata(context.Background(), fwresource.MetadataRequest{
			ProviderTypeName: "atlassian",
		}, resp)
		if resp.TypeName == "atlassian_jira_issue_type_screen_scheme" {
			if _, ok := r.(fwresource.ResourceWithImportState); !ok {
				t.Fatal("resource does not implement ImportState")
			}
			return
		}
	}
	t.Fatal("resource not found")
}

// TestIssue91_SchemaHasMappings verifies the schema includes
// issue_type_mappings attribute.
func TestIssue91_SchemaHasMappings(t *testing.T) {
	t.Parallel()
	p := provider.New("test")()
	resources := p.Resources(context.Background())
	for _, rf := range resources {
		r := rf()
		resp := &fwresource.MetadataResponse{}
		r.Metadata(context.Background(), fwresource.MetadataRequest{
			ProviderTypeName: "atlassian",
		}, resp)
		if resp.TypeName == "atlassian_jira_issue_type_screen_scheme" {
			schemaResp := &fwresource.SchemaResponse{}
			r.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
			if _, ok := schemaResp.Schema.Attributes["issue_type_mappings"]; !ok {
				t.Fatal("schema missing issue_type_mappings attribute")
			}
			return
		}
	}
	t.Fatal("resource not found")
}
