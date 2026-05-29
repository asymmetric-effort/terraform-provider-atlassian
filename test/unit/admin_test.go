// Package unit contains unit tests for the atlassian_organization and
// atlassian_product admin resources and data sources.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	orgdatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/admin/organization"
	proddatasource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/datasources/admin/product"
	orgresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/admin/organization"
	prodresource "github.com/asymmetric-effort/terraform-provider-atlassian/internal/resources/admin/product"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// testAdminMockServer creates a mock HTTP server for admin API endpoints.
// It handles organization reads, product provisioning, status polling, and workspace queries.
func testAdminMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex

	orgs := map[string]map[string]interface{}{
		"org-1": {
			"id":   "org-1",
			"name": "Test Organization",
			"type": "organization",
		},
	}

	// Track provisioning requests and workspaces
	provisionRequests := make(map[string]string) // requestID -> status
	workspaces := map[string][]map[string]interface{}{
		"org-1": {
			{
				"id":   "ws-1",
				"name": "my-site",
				"url":  "https://my-site.atlassian.net",
			},
		},
	}

	mux := http.NewServeMux()

	// GET /v1/orgs/{id} - read organization
	mux.HandleFunc("GET /v1/orgs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mu.Lock()
		defer mu.Unlock()
		org, ok := orgs[id]
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Organization not found"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": org})
	})

	// POST /installations/v2/orgs/{orgID}/products - provision product
	mux.HandleFunc("POST /installations/v2/orgs/{orgID}/products", func(w http.ResponseWriter, r *http.Request) {
		orgID := r.PathValue("orgID")
		mu.Lock()
		defer mu.Unlock()

		if _, ok := orgs[orgID]; !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Organization not found"},
			})
			return
		}

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)

		// Extract site name from parameters
		params, _ := req["parameters"].(map[string]interface{})
		siteName, _ := params["name"].(string)

		requestID := fmt.Sprintf("req-%s-%s", orgID, siteName)
		provisionRequests[requestID] = "COMPLETED"

		// Auto-create workspace
		workspaces[orgID] = append(workspaces[orgID], map[string]interface{}{
			"id":   fmt.Sprintf("ws-%s", siteName),
			"name": siteName,
			"url":  fmt.Sprintf("https://%s.atlassian.net", siteName),
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"requestId": requestID,
			"statusUrl": fmt.Sprintf("/installations/v2/orgs/%s/products/status/%s", orgID, requestID),
		})
	})

	// GET /installations/v2/orgs/{orgID}/products/status/{requestID} - poll status
	mux.HandleFunc("GET /installations/v2/orgs/{orgID}/products/status/{requestID}", func(w http.ResponseWriter, r *http.Request) {
		requestID := r.PathValue("requestID")
		mu.Lock()
		defer mu.Unlock()

		status, ok := provisionRequests[requestID]
		if !ok {
			status = "COMPLETED"
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"requestId": requestID,
				"status":    status,
			},
		})
	})

	// POST /v2/orgs/{orgID}/workspaces - query workspaces
	mux.HandleFunc("POST /v2/orgs/{orgID}/workspaces", func(w http.ResponseWriter, r *http.Request) {
		orgID := r.PathValue("orgID")
		mu.Lock()
		defer mu.Unlock()

		var req struct {
			Query struct {
				Field struct {
					Name   string   `json:"name"`
					Values []string `json:"values"`
				} `json:"field"`
			} `json:"query"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		var data []map[string]interface{}
		for _, ws := range workspaces[orgID] {
			name := ws["name"].(string)
			for _, v := range req.Query.Field.Values {
				if name == v {
					data = append(data, map[string]interface{}{
						"id": ws["id"],
						"attributes": map[string]interface{}{
							"name": ws["name"],
							"url":  ws["url"],
						},
					})
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": data,
		})
	})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// testAdminClient creates a client configured with AdminBaseURL pointing at the test server.
func testAdminClient(t *testing.T, ts *httptest.Server) *atlassian.Client {
	t.Helper()
	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, err := atlassian.NewClient(cfg, &testNoopAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

// ==================== ORGANIZATION RESOURCE ====================

// TestAdminOrganizationResourceMetadata verifies the resource type name.
func TestAdminOrganizationResourceMetadata(t *testing.T) {
	t.Parallel()
	r := orgresource.NewResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_organization" {
		t.Errorf("expected type name 'atlassian_organization', got %q", resp.TypeName)
	}
}

// TestAdminOrganizationResourceSchema verifies the schema has required attributes.
func TestAdminOrganizationResourceSchema(t *testing.T) {
	t.Parallel()
	r := orgresource.NewResource()
	s := getResourceSchema(t, r)
	for _, attr := range []string{"id", "name", "type"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestAdminOrganizationResourceImportState verifies ImportState is implemented.
func TestAdminOrganizationResourceImportState(t *testing.T) {
	t.Parallel()
	r := orgresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Fatal("resource does not implement ImportState")
	}
}

// TestAdminOrganizationResourceCreate tests adopting an existing organization.
func TestAdminOrganizationResourceCreate(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-1"),
		"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, createResp.State, "id"); id != "org-1" {
		t.Errorf("expected id 'org-1', got %q", id)
	}
	if name := getStringAttr(t, createResp.State, "name"); name != "Test Organization" {
		t.Errorf("expected name 'Test Organization', got %q", name)
	}
	if typ := getStringAttr(t, createResp.State, "type"); typ != "organization" {
		t.Errorf("expected type 'organization', got %q", typ)
	}
}

// TestAdminOrganizationResourceCreateNotFound tests adopting a nonexistent organization.
func TestAdminOrganizationResourceCreateNotFound(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "nonexistent"),
		"name": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"type": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if !createResp.Diagnostics.HasError() {
		t.Fatal("Expected error creating with nonexistent org")
	}
}

// TestAdminOrganizationResourceRead tests reading an existing organization.
func TestAdminOrganizationResourceRead(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-1"),
		"name": tftypes.NewValue(tftypes.String, "Old Name"),
		"type": tftypes.NewValue(tftypes.String, "old"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, readResp.State, "name"); name != "Test Organization" {
		t.Errorf("expected name 'Test Organization', got %q", name)
	}
	if typ := getStringAttr(t, readResp.State, "type"); typ != "organization" {
		t.Errorf("expected type 'organization', got %q", typ)
	}
}

// TestAdminOrganizationResourceReadNotFound tests that reading a nonexistent org removes state.
func TestAdminOrganizationResourceReadNotFound(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "nonexistent"),
		"name": tftypes.NewValue(tftypes.String, "X"),
		"type": tftypes.NewValue(tftypes.String, "X"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed after 404")
	}
}

// TestAdminOrganizationResourceDelete tests that delete removes from state only.
func TestAdminOrganizationResourceDelete(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-1"),
		"name": tftypes.NewValue(tftypes.String, "Test Organization"),
		"type": tftypes.NewValue(tftypes.String, "organization"),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
	// Delete should produce a warning but not an error
	if deleteResp.Diagnostics.WarningsCount() == 0 {
		t.Error("expected warning about organization not being deleted from Atlassian")
	}
}

// TestAdminOrganizationResourceUpdate tests that update returns an error.
func TestAdminOrganizationResourceUpdate(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-1"),
		"name": tftypes.NewValue(tftypes.String, "New Name"),
		"type": tftypes.NewValue(tftypes.String, "organization"),
	})}
	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-1"),
		"name": tftypes.NewValue(tftypes.String, "Test Organization"),
		"type": tftypes.NewValue(tftypes.String, "organization"),
	})}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, updateResp)
	if !updateResp.Diagnostics.HasError() {
		t.Fatal("Expected error on update (not supported)")
	}
}

// TestAdminOrganizationResourceConfigureNil verifies nil provider data does not error.
func TestAdminOrganizationResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := orgresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestAdminOrganizationResourceConfigureBadType verifies wrong provider data type errors.
func TestAdminOrganizationResourceConfigureBadType(t *testing.T) {
	t.Parallel()
	r := orgresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// TestAdminOrganizationResourceCreateBadPlan tests Create with invalid plan.
func TestAdminOrganizationResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil plan")
	}
}

// TestAdminOrganizationResourceReadBadState tests Read with invalid state.
func TestAdminOrganizationResourceReadBadState(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.ReadResponse{State: emptyState(ctx, s)}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil state")
	}
}

// TestAdminOrganizationResourceDeleteBadState tests Delete with invalid state.
// Organization delete does not read state, so nil state does not produce an error.
func TestAdminOrganizationResourceDeleteBadState(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.DeleteResponse{State: emptyState(ctx, s)}
	r.Delete(ctx, resource.DeleteRequest{State: state}, resp)
	// Delete does not read state, so nil state should not cause an error
	if resp.Diagnostics.HasError() {
		t.Fatal("Delete should not error even with nil state (it does not read state)")
	}
}

// ==================== PRODUCT RESOURCE ====================

// TestAdminProductResourceMetadata verifies the resource type name.
func TestAdminProductResourceMetadata(t *testing.T) {
	t.Parallel()
	r := prodresource.NewResource()
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_product" {
		t.Errorf("expected type name 'atlassian_product', got %q", resp.TypeName)
	}
}

// TestAdminProductResourceSchema verifies the schema has required attributes.
func TestAdminProductResourceSchema(t *testing.T) {
	t.Parallel()
	r := prodresource.NewResource()
	s := getResourceSchema(t, r)
	for _, attr := range []string{"id", "org_id", "offering_id", "site_name", "location", "admin_email", "timezone", "site_url", "status", "request_id"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestAdminProductResourceImportState verifies ImportState is implemented.
func TestAdminProductResourceImportState(t *testing.T) {
	t.Parallel()
	r := prodresource.NewResource()
	if _, ok := r.(resource.ResourceWithImportState); !ok {
		t.Fatal("resource does not implement ImportState")
	}
}

// TestAdminProductResourceCreate tests provisioning a product, polling status, and finding the workspace.
func TestAdminProductResourceCreate(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "jira-software-offering"),
		"site_name":   tftypes.NewValue(tftypes.String, "new-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@example.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, createResp.State, "id"); id != "ws-new-site" {
		t.Errorf("expected id 'ws-new-site', got %q", id)
	}
	if siteURL := getStringAttr(t, createResp.State, "site_url"); siteURL != "https://new-site.atlassian.net" {
		t.Errorf("expected site_url 'https://new-site.atlassian.net', got %q", siteURL)
	}
	if status := getStringAttr(t, createResp.State, "status"); status != "COMPLETED" {
		t.Errorf("expected status 'COMPLETED', got %q", status)
	}
	if reqID := getStringAttr(t, createResp.State, "request_id"); reqID != "req-org-1-new-site" {
		t.Errorf("expected request_id 'req-org-1-new-site', got %q", reqID)
	}
}

// TestAdminProductResourceRead tests reading an existing workspace.
func TestAdminProductResourceRead(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ws-1"),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "jira-software-offering"),
		"site_name":   tftypes.NewValue(tftypes.String, "my-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@example.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, "https://my-site.atlassian.net"),
		"status":      tftypes.NewValue(tftypes.String, "COMPLETED"),
		"request_id":  tftypes.NewValue(tftypes.String, "req-1"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read: %v", readResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, readResp.State, "id"); id != "ws-1" {
		t.Errorf("expected id 'ws-1', got %q", id)
	}
	if siteURL := getStringAttr(t, readResp.State, "site_url"); siteURL != "https://my-site.atlassian.net" {
		t.Errorf("expected site_url 'https://my-site.atlassian.net', got %q", siteURL)
	}
	if status := getStringAttr(t, readResp.State, "status"); status != "COMPLETED" {
		t.Errorf("expected status 'COMPLETED', got %q", status)
	}
}

// TestAdminProductResourceReadNotFound tests that reading a nonexistent workspace removes state.
func TestAdminProductResourceReadNotFound(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ws-missing"),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "jira-software-offering"),
		"site_name":   tftypes.NewValue(tftypes.String, "missing-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@example.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, "https://missing-site.atlassian.net"),
		"status":      tftypes.NewValue(tftypes.String, "COMPLETED"),
		"request_id":  tftypes.NewValue(tftypes.String, "req-1"),
	})}
	readResp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatalf("Read of nonexistent should not error: %v", readResp.Diagnostics.Errors())
	}
	if !readResp.State.Raw.IsNull() {
		t.Error("expected state to be removed when workspace not found")
	}
}

// TestAdminProductResourceDelete tests that delete removes from state only with a warning.
func TestAdminProductResourceDelete(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ws-1"),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "jira-software-offering"),
		"site_name":   tftypes.NewValue(tftypes.String, "my-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@example.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, "https://my-site.atlassian.net"),
		"status":      tftypes.NewValue(tftypes.String, "COMPLETED"),
		"request_id":  tftypes.NewValue(tftypes.String, "req-1"),
	})}
	deleteResp := &resource.DeleteResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Delete(ctx, resource.DeleteRequest{State: state}, deleteResp)
	if deleteResp.Diagnostics.HasError() {
		t.Fatalf("Delete: %v", deleteResp.Diagnostics.Errors())
	}
	// Delete should produce a warning about product not being deprovisioned
	if deleteResp.Diagnostics.WarningsCount() == 0 {
		t.Error("expected warning about product not being deprovisioned from Atlassian")
	}
}

// TestAdminProductResourceConfigureNil verifies nil provider data does not error.
func TestAdminProductResourceConfigureNil(t *testing.T) {
	t.Parallel()
	r := prodresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestAdminProductResourceConfigureBadType verifies wrong provider data type errors.
func TestAdminProductResourceConfigureBadType(t *testing.T) {
	t.Parallel()
	r := prodresource.NewResource()
	rc := r.(resourceWithConfigure)
	resp := &resource.ConfigureResponse{}
	rc.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// ==================== ORGANIZATION DATA SOURCE ====================

// TestAdminOrganizationDataSourceMetadata verifies the data source type name.
func TestAdminOrganizationDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := orgdatasource.NewDataSource()
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_organization" {
		t.Errorf("expected type name 'atlassian_organization', got %q", resp.TypeName)
	}
}

// TestAdminOrganizationDataSourceSchema verifies the schema has required attributes.
func TestAdminOrganizationDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := orgdatasource.NewDataSource()
	s := getDatasourceSchema(t, ds)
	for _, attr := range []string{"id", "name", "type"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestAdminOrganizationDataSourceRead tests reading an organization via data source.
func TestAdminOrganizationDataSourceRead(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()

	ds := orgdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-1"),
		"name": tftypes.NewValue(tftypes.String, nil),
		"type": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if name := getStringAttr(t, dsResp.State, "name"); name != "Test Organization" {
		t.Errorf("expected name 'Test Organization', got %q", name)
	}
	if typ := getStringAttr(t, dsResp.State, "type"); typ != "organization" {
		t.Errorf("expected type 'organization', got %q", typ)
	}
}

// TestAdminOrganizationDataSourceReadNotFound tests reading a nonexistent organization.
func TestAdminOrganizationDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()

	ds := orgdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "nonexistent"),
		"name": tftypes.NewValue(tftypes.String, nil),
		"type": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent organization")
	}
}

// TestAdminOrganizationDataSourceConfigureNil verifies nil provider data does not error.
func TestAdminOrganizationDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := orgdatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestAdminOrganizationDataSourceConfigureBadType verifies wrong provider data type errors.
func TestAdminOrganizationDataSourceConfigureBadType(t *testing.T) {
	t.Parallel()
	ds := orgdatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// ==================== PRODUCT DATA SOURCE ====================

// TestAdminProductDataSourceMetadata verifies the data source type name.
func TestAdminProductDataSourceMetadata(t *testing.T) {
	t.Parallel()
	ds := proddatasource.NewDataSource()
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "atlassian"}, resp)
	if resp.TypeName != "atlassian_product" {
		t.Errorf("expected type name 'atlassian_product', got %q", resp.TypeName)
	}
}

// TestAdminProductDataSourceSchema verifies the schema has required attributes.
func TestAdminProductDataSourceSchema(t *testing.T) {
	t.Parallel()
	ds := proddatasource.NewDataSource()
	s := getDatasourceSchema(t, ds)
	for _, attr := range []string{"org_id", "site_name", "id", "site_url"} {
		if _, ok := s.Attributes[attr]; !ok {
			t.Errorf("schema missing attribute %q", attr)
		}
	}
}

// TestAdminProductDataSourceRead tests reading a workspace via data source.
func TestAdminProductDataSourceRead(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()

	ds := proddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"org_id":    tftypes.NewValue(tftypes.String, "org-1"),
		"site_name": tftypes.NewValue(tftypes.String, "my-site"),
		"id":        tftypes.NewValue(tftypes.String, nil),
		"site_url":  tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if dsResp.Diagnostics.HasError() {
		t.Fatalf("DS Read: %v", dsResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, dsResp.State, "id"); id != "ws-1" {
		t.Errorf("expected id 'ws-1', got %q", id)
	}
	if siteURL := getStringAttr(t, dsResp.State, "site_url"); siteURL != "https://my-site.atlassian.net" {
		t.Errorf("expected site_url 'https://my-site.atlassian.net', got %q", siteURL)
	}
}

// TestAdminProductDataSourceReadNotFound tests reading a nonexistent workspace.
func TestAdminProductDataSourceReadNotFound(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()

	ds := proddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"org_id":    tftypes.NewValue(tftypes.String, "org-1"),
		"site_name": tftypes.NewValue(tftypes.String, "nonexistent-site"),
		"id":        tftypes.NewValue(tftypes.String, nil),
		"site_url":  tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error reading nonexistent workspace")
	}
}

// TestAdminProductDataSourceConfigureNil verifies nil provider data does not error.
func TestAdminProductDataSourceConfigureNil(t *testing.T) {
	t.Parallel()
	ds := proddatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: nil}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal("nil should not error")
	}
}

// TestAdminProductDataSourceConfigureBadType verifies wrong provider data type errors.
func TestAdminProductDataSourceConfigureBadType(t *testing.T) {
	t.Parallel()
	ds := proddatasource.NewDataSource()
	dc := ds.(datasourceWithConfigure)
	resp := &datasource.ConfigureResponse{}
	dc.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "bad"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with wrong type")
	}
}

// TestAdminOrganizationResourceImportStateExec tests actual ImportState execution.
func TestAdminOrganizationResourceImportStateExec(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	importResp := &resource.ImportStateResponse{State: emptyState(context.Background(), s)}
	r.(resource.ResourceWithImportState).ImportState(context.Background(), resource.ImportStateRequest{ID: "org-abc"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, importResp.State, "id"); id != "org-abc" {
		t.Errorf("expected imported id 'org-abc', got %q", id)
	}
}

// TestAdminProductResourceImportStateExec tests actual ImportState execution.
func TestAdminProductResourceImportStateExec(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	importResp := &resource.ImportStateResponse{State: emptyState(context.Background(), s)}
	r.(resource.ResourceWithImportState).ImportState(context.Background(), resource.ImportStateRequest{ID: "ws-123"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatalf("ImportState: %v", importResp.Diagnostics.Errors())
	}
	if id := getStringAttr(t, importResp.State, "id"); id != "ws-123" {
		t.Errorf("expected imported id 'ws-123', got %q", id)
	}
}

// TestAdminProductResourceUpdate tests that Update refreshes state.
func TestAdminProductResourceUpdate(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	// First create a product to have a workspace available
	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "offering-1"),
		"site_name":   tftypes.NewValue(tftypes.String, "update-test-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	createResp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, createResp)
	if createResp.Diagnostics.HasError() {
		t.Fatalf("Create: %v", createResp.Diagnostics.Errors())
	}
	wsID := getStringAttr(t, createResp.State, "id")

	// Now update (should refresh state from workspaces)
	updatePlan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, wsID),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "offering-1"),
		"site_name":   tftypes.NewValue(tftypes.String, "update-test-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin-new@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "US/Eastern"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	updateState := tfsdk.State{Schema: s, Raw: createResp.State.Raw.Copy()}
	updateResp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: updatePlan, State: updateState}, updateResp)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("Update: %v", updateResp.Diagnostics.Errors())
	}
}

// TestAdminProductResourceUpdateNotFound tests Update when workspace is gone.
func TestAdminProductResourceUpdateNotFound(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, "ws-nonexist"),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "offering-1"),
		"site_name":   tftypes.NewValue(tftypes.String, "nonexistent-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, ""),
		"status":      tftypes.NewValue(tftypes.String, ""),
		"request_id":  tftypes.NewValue(tftypes.String, ""),
	})}
	state := tfsdk.State{Schema: s, Raw: plan.Raw.Copy()}
	resp := &resource.UpdateResponse{State: emptyState(ctx, s)}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error updating nonexistent workspace")
	}
}

// TestAdminProductResourceCreateBadPlan tests Create with nil plan.
func TestAdminProductResourceCreateBadPlan(t *testing.T) {
	t.Parallel()
	ts := testAdminMockServer(t)
	client := testAdminClient(t, ts)
	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(s.Type().TerraformType(ctx), nil)}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error with nil plan")
	}
}

// TestAdminProductResourceCreateForbidden tests 403 on provision.
func TestAdminProductResourceCreateForbidden(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Permission denied"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "offering-1"),
		"site_name":   tftypes.NewValue(tftypes.String, "forbidden-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected permission denied error")
	}
}

// TestAdminOrganizationResourceCreateServerError tests 500 on org read.
func TestAdminOrganizationResourceCreateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-err"),
		"name": tftypes.NewValue(tftypes.String, nil),
		"type": tftypes.NewValue(tftypes.String, nil),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestAdminOrganizationResourceReadServerError tests 500 on read.
func TestAdminOrganizationResourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := orgresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	state := tfsdk.State{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-err"),
		"name": tftypes.NewValue(tftypes.String, ""),
		"type": tftypes.NewValue(tftypes.String, ""),
	})}
	resp := &resource.ReadResponse{State: tfsdk.State{Schema: s, Raw: state.Raw.Copy()}}
	r.Read(ctx, resource.ReadRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error read")
	}
}

// TestAdminOrganizationDataSourceReadServerError tests 500 on data source read.
func TestAdminOrganizationDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	ds := orgdatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"id":   tftypes.NewValue(tftypes.String, "org-err"),
		"name": tftypes.NewValue(tftypes.String, nil),
		"type": tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestAdminProductDataSourceReadServerError tests 500 on product data source read.
func TestAdminProductDataSourceReadServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	ds := proddatasource.NewDataSource()
	configureDatasource(t, ds, client)
	dss := getDatasourceSchema(t, ds)
	dsTfType := dss.Type().TerraformType(ctx)

	config := tfsdk.Config{Schema: dss, Raw: tftypes.NewValue(dsTfType, map[string]tftypes.Value{
		"org_id":    tftypes.NewValue(tftypes.String, "org-1"),
		"site_name": tftypes.NewValue(tftypes.String, "test"),
		"id":        tftypes.NewValue(tftypes.String, nil),
		"site_url":  tftypes.NewValue(tftypes.String, nil),
	})}
	dsResp := &datasource.ReadResponse{State: emptyDSState(ctx, dss)}
	ds.Read(ctx, datasource.ReadRequest{Config: config}, dsResp)
	if !dsResp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestAdminProductResourceCreateProvisionFailed tests FAILED provisioning status.
func TestAdminProductResourceCreateProvisionFailed(t *testing.T) {
	t.Parallel()
	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /installations/v2/orgs/{orgID}/products", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"requestId": "req-fail",
			"statusUrl": "/status/req-fail",
		})
	})
	mux.HandleFunc("GET /installations/v2/orgs/{orgID}/products/status/{reqID}", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"requestId": "req-fail",
				"status":    "FAILED",
			},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "offering-1"),
		"site_name":   tftypes.NewValue(tftypes.String, "fail-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for FAILED provisioning")
	}
}

// TestAdminProductResourceCreateStatusCheckError tests error during status polling.
func TestAdminProductResourceCreateStatusCheckError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /installations/v2/orgs/{orgID}/products", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"requestId": "req-err",
			"statusUrl": "/status/req-err",
		})
	})
	mux.HandleFunc("GET /installations/v2/orgs/{orgID}/products/status/{reqID}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal error"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "offering-1"),
		"site_name":   tftypes.NewValue(tftypes.String, "err-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error for status check failure")
	}
}

// TestAdminProductResourceCreateServerError tests 500 on provision.
func TestAdminProductResourceCreateServerError(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Internal server error"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "offering-1"),
		"site_name":   tftypes.NewValue(tftypes.String, "err-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected error on server error")
	}
}

// TestAdminProductResourceCreateBadRequest tests 400 on provision.
func TestAdminProductResourceCreateBadRequest(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Bad request"},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	cfg := atlassian.Config{
		AdminBaseURL:   ts.URL,
		RequestTimeout: 5000000000,
		MaxRetries:     0,
		RetryWaitMin:   1000000000,
		RetryWaitMax:   1000000000,
	}
	client, _ := atlassian.NewClient(cfg, &testNoopAuth{})

	ctx := context.Background()
	r := prodresource.NewResource()
	configureResource(t, r, client)
	s := getResourceSchema(t, r)
	tfType := s.Type().TerraformType(ctx)

	plan := tfsdk.Plan{Schema: s, Raw: tftypes.NewValue(tfType, map[string]tftypes.Value{
		"id":          tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"org_id":      tftypes.NewValue(tftypes.String, "org-1"),
		"offering_id": tftypes.NewValue(tftypes.String, "bad-offering"),
		"site_name":   tftypes.NewValue(tftypes.String, "bad-site"),
		"location":    tftypes.NewValue(tftypes.String, "us"),
		"admin_email": tftypes.NewValue(tftypes.String, "admin@test.com"),
		"timezone":    tftypes.NewValue(tftypes.String, "UTC"),
		"site_url":    tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"status":      tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		"request_id":  tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})}
	resp := &resource.CreateResponse{State: emptyState(ctx, s)}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("Expected bad request error")
	}
}
