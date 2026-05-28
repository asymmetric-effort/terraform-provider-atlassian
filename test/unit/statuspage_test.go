// Package unit contains unit tests for Statuspage resources and data sources.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	spcomponentds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/component"
	sppageds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/page"
	spsubscriberds "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/statuspage/subscriber"
	spcomponentrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/component"
	sppagers "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/page"
	spsubscriberrs "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/statuspage/subscriber"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	rsschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// spIDCounter provides unique IDs for statuspage mock server tests.
var spIDCounter uint64

func spNextID(prefix string) string {
	n := atomic.AddUint64(&spIDCounter, 1)
	return fmt.Sprintf("%s-%d", prefix, n)
}

// ============================================================================
// Schema and Interface Tests
// ============================================================================

// TestStatuspagePageResourceInterfaces verifies the page resource satisfies framework interfaces.
func TestStatuspagePageResourceInterfaces(t *testing.T) {
	t.Parallel()
	var r resource.Resource = sppagers.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("page resource does not implement ResourceWithImportState")
	}
}

// TestStatuspagePageResourceSchema verifies schema attributes.
func TestStatuspagePageResourceSchema(t *testing.T) {
	t.Parallel()
	r := sppagers.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Schema.Description == "" {
		t.Error("schema description should not be empty")
	}
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "name", "page_description", "subdomain", "url"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(attrs) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(attrs))
	}
}

// TestStatuspagePageResourceMetadata verifies the type name.
func TestStatuspagePageResourceMetadata(t *testing.T) {
	t.Parallel()
	r := sppagers.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_page" {
		t.Errorf("expected type name atlassian_statuspage_page, got %s", resp.TypeName)
	}
}

// TestStatuspageComponentResourceInterfaces verifies the component resource satisfies framework interfaces.
func TestStatuspageComponentResourceInterfaces(t *testing.T) {
	t.Parallel()
	var r resource.Resource = spcomponentrs.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("component resource does not implement ResourceWithImportState")
	}
}

// TestStatuspageComponentResourceSchema verifies schema attributes.
func TestStatuspageComponentResourceSchema(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "description", "status", "group_id"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(attrs) != 6 {
		t.Errorf("expected 6 attributes, got %d", len(attrs))
	}
}

// TestStatuspageComponentResourceMetadata verifies the type name.
func TestStatuspageComponentResourceMetadata(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_component" {
		t.Errorf("expected type name atlassian_statuspage_component, got %s", resp.TypeName)
	}
}

// TestStatuspageComponentGroupResourceInterfaces verifies the component group resource satisfies framework interfaces.
func TestStatuspageComponentGroupResourceInterfaces(t *testing.T) {
	t.Parallel()
	var r resource.Resource = spcomponentrs.NewGroupResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("component group resource does not implement ResourceWithImportState")
	}
}

// TestStatuspageComponentGroupResourceSchema verifies schema attributes.
func TestStatuspageComponentGroupResourceSchema(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewGroupResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "description"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(attrs) != 4 {
		t.Errorf("expected 4 attributes, got %d", len(attrs))
	}
}

// TestStatuspageComponentGroupResourceMetadata verifies the type name.
func TestStatuspageComponentGroupResourceMetadata(t *testing.T) {
	t.Parallel()
	r := spcomponentrs.NewGroupResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_component_group" {
		t.Errorf("expected type name atlassian_statuspage_component_group, got %s", resp.TypeName)
	}
}

// TestStatuspageSubscriberResourceInterfaces verifies the subscriber resource satisfies framework interfaces.
func TestStatuspageSubscriberResourceInterfaces(t *testing.T) {
	t.Parallel()
	var r resource.Resource = spsubscriberrs.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("subscriber resource does not implement ResourceWithImportState")
	}
}

// TestStatuspageSubscriberResourceSchema verifies schema attributes.
func TestStatuspageSubscriberResourceSchema(t *testing.T) {
	t.Parallel()
	r := spsubscriberrs.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "email", "endpoint", "component_ids"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(attrs) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(attrs))
	}
}

// TestStatuspageSubscriberResourceMetadata verifies the type name.
func TestStatuspageSubscriberResourceMetadata(t *testing.T) {
	t.Parallel()
	r := spsubscriberrs.NewResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_subscriber" {
		t.Errorf("expected type name atlassian_statuspage_subscriber, got %s", resp.TypeName)
	}
}

// TestStatuspageIncidentTemplateResourceInterfaces verifies interfaces.
func TestStatuspageIncidentTemplateResourceInterfaces(t *testing.T) {
	t.Parallel()
	var r resource.Resource = sppagers.NewIncidentTemplateResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("incident template resource does not implement ResourceWithImportState")
	}
}

// TestStatuspageIncidentTemplateResourceSchema verifies schema attributes.
func TestStatuspageIncidentTemplateResourceSchema(t *testing.T) {
	t.Parallel()
	r := sppagers.NewIncidentTemplateResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "title", "body"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(attrs) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(attrs))
	}
}

// TestStatuspageIncidentTemplateResourceMetadata verifies type name.
func TestStatuspageIncidentTemplateResourceMetadata(t *testing.T) {
	t.Parallel()
	r := sppagers.NewIncidentTemplateResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_incident_template" {
		t.Errorf("expected type name atlassian_statuspage_incident_template, got %s", resp.TypeName)
	}
}

// TestStatuspageMaintenanceTemplateResourceInterfaces verifies interfaces.
func TestStatuspageMaintenanceTemplateResourceInterfaces(t *testing.T) {
	t.Parallel()
	var r resource.Resource = sppagers.NewMaintenanceTemplateResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("maintenance template resource does not implement ResourceWithImportState")
	}
}

// TestStatuspageMaintenanceTemplateResourceSchema verifies schema attributes.
func TestStatuspageMaintenanceTemplateResourceSchema(t *testing.T) {
	t.Parallel()
	r := sppagers.NewMaintenanceTemplateResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "title", "body"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(attrs) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(attrs))
	}
}

// TestStatuspageMaintenanceTemplateResourceMetadata verifies type name.
func TestStatuspageMaintenanceTemplateResourceMetadata(t *testing.T) {
	t.Parallel()
	r := sppagers.NewMaintenanceTemplateResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_maintenance_template" {
		t.Errorf("expected type name atlassian_statuspage_maintenance_template, got %s", resp.TypeName)
	}
}

// TestStatuspagePermissionResourceInterfaces verifies interfaces.
func TestStatuspagePermissionResourceInterfaces(t *testing.T) {
	t.Parallel()
	var r resource.Resource = sppagers.NewPermissionResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Error("permission resource does not implement ResourceWithImportState")
	}
}

// TestStatuspagePermissionResourceSchema verifies schema attributes.
func TestStatuspagePermissionResourceSchema(t *testing.T) {
	t.Parallel()
	r := sppagers.NewPermissionResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "principal_type", "principal_id", "role"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
	if len(attrs) != 5 {
		t.Errorf("expected 5 attributes, got %d", len(attrs))
	}
}

// TestStatuspagePermissionResourceMetadata verifies type name.
func TestStatuspagePermissionResourceMetadata(t *testing.T) {
	t.Parallel()
	r := sppagers.NewPermissionResource()
	req := resource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_permission" {
		t.Errorf("expected type name atlassian_statuspage_permission, got %s", resp.TypeName)
	}
}

// ============================================================================
// Data Source Schema and Interface Tests
// ============================================================================

// TestStatuspagePageDataSourceInterfaces verifies data source interfaces.
func TestStatuspagePageDataSourceInterfaces(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = sppageds.NewDataSource()
}

// TestStatuspagePageDataSourceSchema verifies schema attributes.
func TestStatuspagePageDataSourceSchema(t *testing.T) {
	t.Parallel()
	d := sppageds.NewDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "name", "page_description", "subdomain", "url"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

// TestStatuspagePageDataSourceMetadata verifies the type name.
func TestStatuspagePageDataSourceMetadata(t *testing.T) {
	t.Parallel()
	d := sppageds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_page" {
		t.Errorf("expected type name atlassian_statuspage_page, got %s", resp.TypeName)
	}
}

// TestStatuspageComponentDataSourceInterfaces verifies data source interfaces.
func TestStatuspageComponentDataSourceInterfaces(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = spcomponentds.NewDataSource()
}

// TestStatuspageComponentDataSourceSchema verifies schema attributes.
func TestStatuspageComponentDataSourceSchema(t *testing.T) {
	t.Parallel()
	d := spcomponentds.NewDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "description", "status", "group_id"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

// TestStatuspageComponentGroupDataSourceInterfaces verifies data source interfaces.
func TestStatuspageComponentGroupDataSourceInterfaces(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = spcomponentds.NewGroupDataSource()
}

// TestStatuspageComponentGroupDataSourceSchema verifies schema attributes.
func TestStatuspageComponentGroupDataSourceSchema(t *testing.T) {
	t.Parallel()
	d := spcomponentds.NewGroupDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "description"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

// TestStatuspageSubscriberDataSourceInterfaces verifies data source interfaces.
func TestStatuspageSubscriberDataSourceInterfaces(t *testing.T) {
	t.Parallel()
	var _ datasource.DataSource = spsubscriberds.NewDataSource()
}

// TestStatuspageSubscriberDataSourceSchema verifies schema attributes.
func TestStatuspageSubscriberDataSourceSchema(t *testing.T) {
	t.Parallel()
	d := spsubscriberds.NewDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "email", "endpoint", "component_ids"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

// TestStatuspageIncidentTemplateDataSourceSchema verifies schema attributes.
func TestStatuspageIncidentTemplateDataSourceSchema(t *testing.T) {
	t.Parallel()
	d := sppageds.NewIncidentTemplateDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "title", "body"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

// TestStatuspageMaintenanceTemplateDataSourceSchema verifies schema attributes.
func TestStatuspageMaintenanceTemplateDataSourceSchema(t *testing.T) {
	t.Parallel()
	d := sppageds.NewMaintenanceTemplateDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "name", "title", "body"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

// TestStatuspagePermissionDataSourceSchema verifies schema attributes.
func TestStatuspagePermissionDataSourceSchema(t *testing.T) {
	t.Parallel()
	d := sppageds.NewPermissionDataSource()
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	attrs := resp.Schema.Attributes
	for _, name := range []string{"id", "page_id", "principal_type", "principal_id", "role"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}
}

// TestStatuspageIncidentTemplateDataSourceMetadata verifies type name.
func TestStatuspageIncidentTemplateDataSourceMetadata(t *testing.T) {
	t.Parallel()
	d := sppageds.NewIncidentTemplateDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_incident_template" {
		t.Errorf("expected atlassian_statuspage_incident_template, got %s", resp.TypeName)
	}
}

// TestStatuspageMaintenanceTemplateDataSourceMetadata verifies type name.
func TestStatuspageMaintenanceTemplateDataSourceMetadata(t *testing.T) {
	t.Parallel()
	d := sppageds.NewMaintenanceTemplateDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_maintenance_template" {
		t.Errorf("expected atlassian_statuspage_maintenance_template, got %s", resp.TypeName)
	}
}

// TestStatuspagePermissionDataSourceMetadata verifies type name.
func TestStatuspagePermissionDataSourceMetadata(t *testing.T) {
	t.Parallel()
	d := sppageds.NewPermissionDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_permission" {
		t.Errorf("expected atlassian_statuspage_permission, got %s", resp.TypeName)
	}
}

// TestStatuspageComponentDataSourceMetadata verifies type name.
func TestStatuspageComponentDataSourceMetadata(t *testing.T) {
	t.Parallel()
	d := spcomponentds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_component" {
		t.Errorf("expected atlassian_statuspage_component, got %s", resp.TypeName)
	}
}

// TestStatuspageComponentGroupDataSourceMetadata verifies type name.
func TestStatuspageComponentGroupDataSourceMetadata(t *testing.T) {
	t.Parallel()
	d := spcomponentds.NewGroupDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_component_group" {
		t.Errorf("expected atlassian_statuspage_component_group, got %s", resp.TypeName)
	}
}

// TestStatuspageSubscriberDataSourceMetadata verifies type name.
func TestStatuspageSubscriberDataSourceMetadata(t *testing.T) {
	t.Parallel()
	d := spsubscriberds.NewDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "atlassian"}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), req, resp)
	if resp.TypeName != "atlassian_statuspage_subscriber" {
		t.Errorf("expected atlassian_statuspage_subscriber, got %s", resp.TypeName)
	}
}

// ============================================================================
// Configure nil tests (all resources + data sources)
// ============================================================================

// TestStatuspageResourceConfigureNil tests that all resources handle nil provider data.
func TestStatuspageResourceConfigureNil(t *testing.T) {
	t.Parallel()
	resources := []resource.Resource{
		sppagers.NewResource(),
		sppagers.NewIncidentTemplateResource(),
		sppagers.NewMaintenanceTemplateResource(),
		sppagers.NewPermissionResource(),
		spcomponentrs.NewResource(),
		spcomponentrs.NewGroupResource(),
		spsubscriberrs.NewResource(),
	}
	for _, r := range resources {
		rc, ok := r.(resource.ResourceWithConfigure)
		if !ok {
			continue
		}
		resp := &resource.ConfigureResponse{}
		rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("configure with nil should not error: %v", resp.Diagnostics.Errors())
		}
	}
}

// TestStatuspageResourceConfigureWrongType tests that all resources handle wrong provider data type.
func TestStatuspageResourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	resources := []resource.Resource{
		sppagers.NewResource(),
		sppagers.NewIncidentTemplateResource(),
		sppagers.NewMaintenanceTemplateResource(),
		sppagers.NewPermissionResource(),
		spcomponentrs.NewResource(),
		spcomponentrs.NewGroupResource(),
		spsubscriberrs.NewResource(),
	}
	for _, r := range resources {
		rc, ok := r.(resource.ResourceWithConfigure)
		if !ok {
			continue
		}
		resp := &resource.ConfigureResponse{}
		rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "wrong"}, resp)
		if !resp.Diagnostics.HasError() {
			t.Error("configure with wrong type should error")
		}
	}
}

// TestStatuspageDataSourceConfigureNil tests nil provider data handling for data sources.
func TestStatuspageDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	dataSources := []datasource.DataSource{
		sppageds.NewDataSource(),
		sppageds.NewIncidentTemplateDataSource(),
		sppageds.NewMaintenanceTemplateDataSource(),
		sppageds.NewPermissionDataSource(),
		spcomponentds.NewDataSource(),
		spcomponentds.NewGroupDataSource(),
		spsubscriberds.NewDataSource(),
	}
	for _, d := range dataSources {
		dc, ok := d.(datasource.DataSourceWithConfigure)
		if !ok {
			continue
		}
		resp := &datasource.ConfigureResponse{}
		dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("configure with nil should not error: %v", resp.Diagnostics.Errors())
		}
	}
}

// TestStatuspageDataSourceConfigureWrongType tests wrong provider data type for data sources.
func TestStatuspageDataSourceConfigureWrongType(t *testing.T) {
	t.Parallel()
	dataSources := []datasource.DataSource{
		sppageds.NewDataSource(),
		sppageds.NewIncidentTemplateDataSource(),
		sppageds.NewMaintenanceTemplateDataSource(),
		sppageds.NewPermissionDataSource(),
		spcomponentds.NewDataSource(),
		spcomponentds.NewGroupDataSource(),
		spsubscriberds.NewDataSource(),
	}
	for _, d := range dataSources {
		dc, ok := d.(datasource.DataSourceWithConfigure)
		if !ok {
			continue
		}
		resp := &datasource.ConfigureResponse{}
		dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "wrong"}, resp)
		if !resp.Diagnostics.HasError() {
			t.Error("configure with wrong type should error")
		}
	}
}

// ============================================================================
// CRUD tests against mock server
// ============================================================================

// testStatuspageMockServer creates a mock HTTP server for Statuspage resources.
func testStatuspageMockServer(t *testing.T) (*httptest.Server, *atlassian.Client) {
	t.Helper()
	var mu sync.Mutex
	pages := make(map[string]map[string]interface{})
	components := make(map[string]map[string]interface{})
	componentGroups := make(map[string]map[string]interface{})
	subscribers := make(map[string]map[string]interface{})
	incidentTemplates := make(map[string]map[string]interface{})
	maintenanceTemplates := make(map[string]map[string]interface{})
	permissions := make(map[string]map[string]interface{})

	mux := http.NewServeMux()

	// ========== PAGE ENDPOINTS ==========
	mux.HandleFunc("POST /v1/pages", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		pageData, _ := req["page"].(map[string]interface{})
		if pageData == nil {
			writeErr(w, 400, "Missing page field")
			return
		}
		name, _ := pageData["name"].(string)
		if name == "" {
			writeErr(w, 400, "name is required")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := spNextID("sp-page")
		subdomain, _ := pageData["subdomain"].(string)
		desc, _ := pageData["page_description"].(string)
		page := map[string]interface{}{
			"id":               id,
			"name":             name,
			"page_description": desc,
			"subdomain":        subdomain,
			"url":              fmt.Sprintf("https://%s.statuspage.io", subdomain),
		}
		pages[id] = page
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(page)
	})

	mux.HandleFunc("GET /v1/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		page, ok := pages[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Page not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
	})

	mux.HandleFunc("PUT /v1/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		pageData, _ := req["page"].(map[string]interface{})
		mu.Lock()
		defer mu.Unlock()
		page, ok := pages[id]
		if !ok {
			writeErr(w, 404, "Page not found")
			return
		}
		if pageData != nil {
			for k, v := range pageData {
				if k != "id" {
					page[k] = v
				}
			}
		}
		pages[id] = page
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(page)
	})

	mux.HandleFunc("DELETE /v1/pages/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := pages[id]; !ok {
			writeErr(w, 404, "Page not found")
			return
		}
		delete(pages, id)
		w.WriteHeader(204)
	})

	// ========== COMPONENT ENDPOINTS ==========
	mux.HandleFunc("POST /v1/pages/{page_id}/components", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		compData, _ := req["component"].(map[string]interface{})
		if compData == nil {
			writeErr(w, 400, "Missing component field")
			return
		}
		name, _ := compData["name"].(string)
		mu.Lock()
		defer mu.Unlock()
		id := spNextID("sp-comp")
		desc, _ := compData["description"].(string)
		status, _ := compData["status"].(string)
		if status == "" {
			status = "operational"
		}
		groupID, _ := compData["group_id"].(string)
		comp := map[string]interface{}{
			"id":          id,
			"page_id":     pageID,
			"name":        name,
			"description": desc,
			"status":      status,
			"group_id":    groupID,
		}
		components[id] = comp
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(comp)
	})

	mux.HandleFunc("GET /v1/pages/{page_id}/components/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		comp, ok := components[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Component not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comp)
	})

	mux.HandleFunc("PUT /v1/pages/{page_id}/components/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		compData, _ := req["component"].(map[string]interface{})
		mu.Lock()
		defer mu.Unlock()
		comp, ok := components[id]
		if !ok {
			writeErr(w, 404, "Component not found")
			return
		}
		if compData != nil {
			for k, v := range compData {
				if k != "id" && k != "page_id" {
					comp[k] = v
				}
			}
		}
		components[id] = comp
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(comp)
	})

	mux.HandleFunc("DELETE /v1/pages/{page_id}/components/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := components[id]; !ok {
			writeErr(w, 404, "Component not found")
			return
		}
		delete(components, id)
		w.WriteHeader(204)
	})

	// ========== COMPONENT GROUP ENDPOINTS ==========
	mux.HandleFunc("POST /v1/pages/{page_id}/component-groups", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		grpData, _ := req["component_group"].(map[string]interface{})
		if grpData == nil {
			writeErr(w, 400, "Missing component_group field")
			return
		}
		name, _ := grpData["name"].(string)
		mu.Lock()
		defer mu.Unlock()
		id := spNextID("sp-cg")
		desc, _ := grpData["description"].(string)
		grp := map[string]interface{}{
			"id":          id,
			"page_id":     pageID,
			"name":        name,
			"description": desc,
		}
		componentGroups[id] = grp
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(grp)
	})

	mux.HandleFunc("GET /v1/pages/{page_id}/component-groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		grp, ok := componentGroups[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Component group not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(grp)
	})

	mux.HandleFunc("PUT /v1/pages/{page_id}/component-groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		grpData, _ := req["component_group"].(map[string]interface{})
		mu.Lock()
		defer mu.Unlock()
		grp, ok := componentGroups[id]
		if !ok {
			writeErr(w, 404, "Component group not found")
			return
		}
		if grpData != nil {
			for k, v := range grpData {
				if k != "id" && k != "page_id" {
					grp[k] = v
				}
			}
		}
		componentGroups[id] = grp
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(grp)
	})

	mux.HandleFunc("DELETE /v1/pages/{page_id}/component-groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := componentGroups[id]; !ok {
			writeErr(w, 404, "Component group not found")
			return
		}
		delete(componentGroups, id)
		w.WriteHeader(204)
	})

	// ========== SUBSCRIBER ENDPOINTS ==========
	mux.HandleFunc("POST /v1/pages/{page_id}/subscribers", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		subData, _ := req["subscriber"].(map[string]interface{})
		if subData == nil {
			writeErr(w, 400, "Missing subscriber field")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := spNextID("sp-sub")
		email, _ := subData["email"].(string)
		endpoint, _ := subData["endpoint"].(string)
		var compIDs []string
		if cids, ok := subData["component_ids"].([]interface{}); ok {
			for _, cid := range cids {
				if s, ok := cid.(string); ok {
					compIDs = append(compIDs, s)
				}
			}
		}
		if compIDs == nil {
			compIDs = []string{}
		}
		sub := map[string]interface{}{
			"id":            id,
			"page_id":       pageID,
			"email":         email,
			"endpoint":      endpoint,
			"component_ids": compIDs,
		}
		subscribers[id] = sub
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(sub)
	})

	mux.HandleFunc("GET /v1/pages/{page_id}/subscribers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		sub, ok := subscribers[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Subscriber not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sub)
	})

	mux.HandleFunc("PUT /v1/pages/{page_id}/subscribers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		subData, _ := req["subscriber"].(map[string]interface{})
		mu.Lock()
		defer mu.Unlock()
		sub, ok := subscribers[id]
		if !ok {
			writeErr(w, 404, "Subscriber not found")
			return
		}
		if subData != nil {
			for k, v := range subData {
				if k != "id" && k != "page_id" {
					sub[k] = v
				}
			}
		}
		subscribers[id] = sub
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sub)
	})

	mux.HandleFunc("DELETE /v1/pages/{page_id}/subscribers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := subscribers[id]; !ok {
			writeErr(w, 404, "Subscriber not found")
			return
		}
		delete(subscribers, id)
		w.WriteHeader(204)
	})

	// ========== INCIDENT TEMPLATE ENDPOINTS ==========
	mux.HandleFunc("POST /v1/pages/{page_id}/incident_templates", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		tmplData, _ := req["template"].(map[string]interface{})
		if tmplData == nil {
			writeErr(w, 400, "Missing template field")
			return
		}
		name, _ := tmplData["name"].(string)
		mu.Lock()
		defer mu.Unlock()
		id := spNextID("sp-it")
		title, _ := tmplData["title"].(string)
		body, _ := tmplData["body"].(string)
		tmpl := map[string]interface{}{
			"id": id, "page_id": pageID, "name": name, "title": title, "body": body,
		}
		incidentTemplates[id] = tmpl
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("GET /v1/pages/{page_id}/incident_templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		tmpl, ok := incidentTemplates[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Incident template not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("PUT /v1/pages/{page_id}/incident_templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		tmplData, _ := req["template"].(map[string]interface{})
		mu.Lock()
		defer mu.Unlock()
		tmpl, ok := incidentTemplates[id]
		if !ok {
			writeErr(w, 404, "Incident template not found")
			return
		}
		if tmplData != nil {
			for k, v := range tmplData {
				if k != "id" && k != "page_id" {
					tmpl[k] = v
				}
			}
		}
		incidentTemplates[id] = tmpl
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("DELETE /v1/pages/{page_id}/incident_templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := incidentTemplates[id]; !ok {
			writeErr(w, 404, "Incident template not found")
			return
		}
		delete(incidentTemplates, id)
		w.WriteHeader(204)
	})

	// ========== MAINTENANCE TEMPLATE ENDPOINTS ==========
	mux.HandleFunc("POST /v1/pages/{page_id}/maintenance_templates", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		tmplData, _ := req["template"].(map[string]interface{})
		if tmplData == nil {
			writeErr(w, 400, "Missing template field")
			return
		}
		name, _ := tmplData["name"].(string)
		mu.Lock()
		defer mu.Unlock()
		id := spNextID("sp-mt")
		title, _ := tmplData["title"].(string)
		body, _ := tmplData["body"].(string)
		tmpl := map[string]interface{}{
			"id": id, "page_id": pageID, "name": name, "title": title, "body": body,
		}
		maintenanceTemplates[id] = tmpl
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("GET /v1/pages/{page_id}/maintenance_templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		tmpl, ok := maintenanceTemplates[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Maintenance template not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("PUT /v1/pages/{page_id}/maintenance_templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		tmplData, _ := req["template"].(map[string]interface{})
		mu.Lock()
		defer mu.Unlock()
		tmpl, ok := maintenanceTemplates[id]
		if !ok {
			writeErr(w, 404, "Maintenance template not found")
			return
		}
		if tmplData != nil {
			for k, v := range tmplData {
				if k != "id" && k != "page_id" {
					tmpl[k] = v
				}
			}
		}
		maintenanceTemplates[id] = tmpl
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tmpl)
	})

	mux.HandleFunc("DELETE /v1/pages/{page_id}/maintenance_templates/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := maintenanceTemplates[id]; !ok {
			writeErr(w, 404, "Maintenance template not found")
			return
		}
		delete(maintenanceTemplates, id)
		w.WriteHeader(204)
	})

	// ========== PERMISSION ENDPOINTS ==========
	mux.HandleFunc("POST /v1/pages/{page_id}/permissions", func(w http.ResponseWriter, r *http.Request) {
		pageID := r.PathValue("page_id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		permData, _ := req["permission"].(map[string]interface{})
		if permData == nil {
			writeErr(w, 400, "Missing permission field")
			return
		}
		mu.Lock()
		defer mu.Unlock()
		id := spNextID("sp-perm")
		pType, _ := permData["principal_type"].(string)
		pID, _ := permData["principal_id"].(string)
		role, _ := permData["role"].(string)
		perm := map[string]interface{}{
			"id": id, "page_id": pageID, "principal_type": pType, "principal_id": pID, "role": role,
		}
		permissions[id] = perm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(perm)
	})

	mux.HandleFunc("GET /v1/pages/{page_id}/permissions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		perm, ok := permissions[id]
		mu.Unlock()
		if !ok {
			writeErr(w, 404, "Permission not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perm)
	})

	mux.HandleFunc("PUT /v1/pages/{page_id}/permissions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		permData, _ := req["permission"].(map[string]interface{})
		mu.Lock()
		defer mu.Unlock()
		perm, ok := permissions[id]
		if !ok {
			writeErr(w, 404, "Permission not found")
			return
		}
		if permData != nil {
			for k, v := range permData {
				if k != "id" && k != "page_id" {
					perm[k] = v
				}
			}
		}
		permissions[id] = perm
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(perm)
	})

	mux.HandleFunc("DELETE /v1/pages/{page_id}/permissions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		if _, ok := permissions[id]; !ok {
			writeErr(w, 404, "Permission not found")
			return
		}
		delete(permissions, id)
		w.WriteHeader(204)
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	auth, err := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}
	cfg := atlassian.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 10 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}
	c, err := atlassian.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	return ts, c
}

// TestStatuspagePageResourceCRUD tests full CRUD lifecycle for the page resource.
func TestStatuspagePageResourceCRUD(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)

	r := sppagers.NewResource()
	r.(resource.ResourceWithConfigure).Configure(context.Background(),
		resource.ConfigureRequest{ProviderData: c},
		&resource.ConfigureResponse{})

	ctx := context.Background()

	// Create
	createResp := &resource.CreateResponse{State: tfsdk.State{Schema: getStatuspagePageSchema()}}
	r.Create(ctx, resource.CreateRequest{
		Plan: tfsdk.Plan{
			Schema: getStatuspagePageSchema(),
			Raw: tftypes.NewValue(getStatuspagePageTfType(), map[string]tftypes.Value{
				"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				"name":             tftypes.NewValue(tftypes.String, "Test Page"),
				"page_description": tftypes.NewValue(tftypes.String, "Test Description"),
				"subdomain":        tftypes.NewValue(tftypes.String, "testpage"),
				"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			}),
		},
	}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create failed: %v", createResp.Diagnostics.Errors())
	}

	// Read
	readResp := &resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("read failed: %v", readResp.Diagnostics.Errors())
	}

	// Update
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: getStatuspagePageSchema()}}
	r.Update(ctx, resource.UpdateRequest{
		State: createResp.State,
		Plan: tfsdk.Plan{
			Schema: getStatuspagePageSchema(),
			Raw: tftypes.NewValue(getStatuspagePageTfType(), map[string]tftypes.Value{
				"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
				"name":             tftypes.NewValue(tftypes.String, "Updated Page"),
				"page_description": tftypes.NewValue(tftypes.String, "Updated Description"),
				"subdomain":        tftypes.NewValue(tftypes.String, "testpage"),
				"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			}),
		},
	}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update failed: %v", updateResp.Diagnostics.Errors())
	}

	// Delete
	deleteResp := &resource.DeleteResponse{}
	r.Delete(ctx, resource.DeleteRequest{State: createResp.State}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("delete failed: %v", deleteResp.Diagnostics.Errors())
	}

	// Read after delete (should remove resource)
	readAfterDelete := &resource.ReadResponse{State: createResp.State}
	r.Read(ctx, resource.ReadRequest{State: createResp.State}, readAfterDelete)
	if readAfterDelete.Diagnostics.HasError() {
		t.Fatalf("read after delete failed: %v", readAfterDelete.Diagnostics.Errors())
	}
}

// TestStatuspagePageResourceErrorNotFound tests error handling for a not-found page during read.
func TestStatuspagePageResourceErrorNotFound(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)

	r := sppagers.NewResource()
	r.(resource.ResourceWithConfigure).Configure(context.Background(),
		resource.ConfigureRequest{ProviderData: c},
		&resource.ConfigureResponse{})

	state := tfsdk.State{
		Schema: getStatuspagePageSchema(),
		Raw: tftypes.NewValue(getStatuspagePageTfType(), map[string]tftypes.Value{
			"id":               tftypes.NewValue(tftypes.String, "nonexistent"),
			"name":             tftypes.NewValue(tftypes.String, "Test"),
			"page_description": tftypes.NewValue(tftypes.String, ""),
			"subdomain":        tftypes.NewValue(tftypes.String, ""),
			"url":              tftypes.NewValue(tftypes.String, ""),
		}),
	}
	readResp := &resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, readResp)
	// Read on missing resource should remove it, not error
	if readResp.Diagnostics.HasError() {
		t.Errorf("read not-found should not error (should remove resource): %v", readResp.Diagnostics.Errors())
	}
}

// TestStatuspagePageResourceErrorUpdateNotFound tests error handling for update on deleted page.
func TestStatuspagePageResourceErrorUpdateNotFound(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)

	r := sppagers.NewResource()
	r.(resource.ResourceWithConfigure).Configure(context.Background(),
		resource.ConfigureRequest{ProviderData: c},
		&resource.ConfigureResponse{})

	state := tfsdk.State{
		Schema: getStatuspagePageSchema(),
		Raw: tftypes.NewValue(getStatuspagePageTfType(), map[string]tftypes.Value{
			"id":               tftypes.NewValue(tftypes.String, "nonexistent"),
			"name":             tftypes.NewValue(tftypes.String, "Test"),
			"page_description": tftypes.NewValue(tftypes.String, ""),
			"subdomain":        tftypes.NewValue(tftypes.String, ""),
			"url":              tftypes.NewValue(tftypes.String, ""),
		}),
	}
	plan := tfsdk.Plan{
		Schema: getStatuspagePageSchema(),
		Raw: tftypes.NewValue(getStatuspagePageTfType(), map[string]tftypes.Value{
			"id":               tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
			"name":             tftypes.NewValue(tftypes.String, "Updated"),
			"page_description": tftypes.NewValue(tftypes.String, ""),
			"subdomain":        tftypes.NewValue(tftypes.String, ""),
			"url":              tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}),
	}
	updateResp := &resource.UpdateResponse{State: tfsdk.State{Schema: getStatuspagePageSchema()}}
	r.Update(context.Background(), resource.UpdateRequest{State: state, Plan: plan}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Error("update on nonexistent page should error")
	}
	found := false
	for _, d := range updateResp.Diagnostics.Errors() {
		if strings.Contains(d.Detail(), "not found") || strings.Contains(d.Summary(), "not found") {
			found = true
		}
	}
	if !found {
		t.Error("error should mention 'not found'")
	}
}

// TestStatuspagePageResourceErrorDeleteNotFound tests delete on already-deleted page.
func TestStatuspagePageResourceErrorDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := testStatuspageMockServer(t)

	r := sppagers.NewResource()
	r.(resource.ResourceWithConfigure).Configure(context.Background(),
		resource.ConfigureRequest{ProviderData: c},
		&resource.ConfigureResponse{})

	state := tfsdk.State{
		Schema: getStatuspagePageSchema(),
		Raw: tftypes.NewValue(getStatuspagePageTfType(), map[string]tftypes.Value{
			"id":               tftypes.NewValue(tftypes.String, "nonexistent"),
			"name":             tftypes.NewValue(tftypes.String, "Test"),
			"page_description": tftypes.NewValue(tftypes.String, ""),
			"subdomain":        tftypes.NewValue(tftypes.String, ""),
			"url":              tftypes.NewValue(tftypes.String, ""),
		}),
	}
	deleteResp := &resource.DeleteResponse{}
	r.Delete(context.Background(), resource.DeleteRequest{State: state}, deleteResp)
	// Deleting a non-existent resource should NOT error (already gone)
	if deleteResp.Diagnostics.HasError() {
		t.Errorf("delete on nonexistent page should not error: %v", deleteResp.Diagnostics.Errors())
	}
}

// ============================================================================
// Schema helpers
// ============================================================================

func getStatuspagePageSchema() rsschema.Schema {
	r := sppagers.NewResource()
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	return resp.Schema
}

func getStatuspagePageTfType() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":               tftypes.String,
			"name":             tftypes.String,
			"page_description": tftypes.String,
			"subdomain":        tftypes.String,
			"url":              tftypes.String,
		},
	}
}

// Unused variable suppression for imports.
var (
	_ = strings.Contains
)
