// Package integration contains comprehensive integration tests for Statuspage
// resources exercised against the mock API server.
//
// These tests verify full CRUD lifecycles, cross-resource operations,
// import patterns, idempotency, drift detection, and error handling for:
// page, component, component_group, subscriber, incident_template,
// maintenance_template, permission.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// setupStatuspageMockServer creates a mock server with Statuspage endpoints.
func setupStatuspageMockServer(t *testing.T) (*httptest.Server, *client.Client) {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterStatuspageEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	auth, err := client.NewAPIKeyAuthenticator("test-api-key")
	if err != nil {
		t.Fatalf("failed to create authenticator: %v", err)
	}

	cfg := client.Config{
		BaseURL:        ts.URL,
		RequestTimeout: testTimeout,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}

	c, err := client.NewClient(cfg, auth)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return ts, c
}

// spBody marshals v to a bytes.Reader for use as a request body.
func spBody(t *testing.T, v interface{}) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}
	return bytes.NewReader(data)
}

// ============================================================================
// Page CRUD Lifecycle
// ============================================================================

// TestStatuspageIntegrationPageCRUDLifecycle tests the full CRUD lifecycle for pages.
func TestStatuspageIntegrationPageCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create
	createBody := map[string]interface{}{
		"page": map[string]interface{}{
			"name":             "Integration Test Page",
			"page_description": "Integration test",
			"subdomain":        "integtest",
		},
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/v1/pages", spBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if created["name"] != "Integration Test Page" {
		t.Errorf("expected name 'Integration Test Page', got %v", created["name"])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s", id), &read)
	if err != nil {
		t.Fatalf("read page failed: %v", err)
	}
	if read["name"] != "Integration Test Page" {
		t.Errorf("read: expected name, got %v", read["name"])
	}

	// Update
	updateBody := map[string]interface{}{
		"page": map[string]interface{}{
			"name":             "Updated Integration Page",
			"page_description": "Updated",
		},
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s", id), spBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update page failed: %v", err)
	}
	if updated["name"] != "Updated Integration Page" {
		t.Errorf("update: expected name 'Updated Integration Page', got %v", updated["name"])
	}

	// Idempotency: re-read
	var reread map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s", id), &reread)
	if err != nil {
		t.Fatalf("re-read page failed: %v", err)
	}
	if reread["name"] != "Updated Integration Page" {
		t.Errorf("re-read: update not persisted")
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/v1/pages/%s", id))
	if err != nil {
		t.Fatalf("delete page failed: %v", err)
	}

	// Verify gone
	var gone map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s", id), &gone)
	if err == nil {
		t.Error("expected error reading deleted page")
	}
}

// ============================================================================
// Component CRUD Lifecycle
// ============================================================================

// TestStatuspageIntegrationComponentCRUDLifecycle tests the full CRUD lifecycle for components.
func TestStatuspageIntegrationComponentCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a page first
	var page map[string]interface{}
	err := c.Post(ctx, "/v1/pages", spBody(t, map[string]interface{}{
		"page": map[string]interface{}{"name": "Component Test Page", "subdomain": "comptest"},
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID, _ := page["id"].(string)

	// Create component
	var comp map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/v1/pages/%s/components", pageID), spBody(t, map[string]interface{}{
		"component": map[string]interface{}{
			"name":        "Database",
			"description": "Primary DB",
			"status":      "operational",
		},
	}), &comp)
	if err != nil {
		t.Fatalf("create component failed: %v", err)
	}
	compID, _ := comp["id"].(string)
	if comp["name"] != "Database" {
		t.Errorf("expected name 'Database', got %v", comp["name"])
	}

	// Read
	var readComp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s/components/%s", pageID, compID), &readComp)
	if err != nil {
		t.Fatalf("read component failed: %v", err)
	}

	// Update
	var updatedComp map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s/components/%s", pageID, compID), spBody(t, map[string]interface{}{
		"component": map[string]interface{}{
			"name":   "Updated Database",
			"status": "degraded_performance",
		},
	}), &updatedComp)
	if err != nil {
		t.Fatalf("update component failed: %v", err)
	}
	if updatedComp["status"] != "degraded_performance" {
		t.Errorf("expected status 'degraded_performance', got %v", updatedComp["status"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/v1/pages/%s/components/%s", pageID, compID))
	if err != nil {
		t.Fatalf("delete component failed: %v", err)
	}
}

// ============================================================================
// Component Group CRUD Lifecycle
// ============================================================================

// TestStatuspageIntegrationComponentGroupCRUDLifecycle tests full CRUD lifecycle.
func TestStatuspageIntegrationComponentGroupCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pageID := "integ-page-cg"

	var group map[string]interface{}
	err := c.Post(ctx, fmt.Sprintf("/v1/pages/%s/component-groups", pageID), spBody(t, map[string]interface{}{
		"component_group": map[string]interface{}{
			"name":        "API Services",
			"description": "All API services",
		},
	}), &group)
	if err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	groupID, _ := group["id"].(string)

	// Read
	var readGroup map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s/component-groups/%s", pageID, groupID), &readGroup)
	if err != nil {
		t.Fatalf("read group failed: %v", err)
	}

	// Update
	var updatedGroup map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s/component-groups/%s", pageID, groupID), spBody(t, map[string]interface{}{
		"component_group": map[string]interface{}{"name": "Updated API Services"},
	}), &updatedGroup)
	if err != nil {
		t.Fatalf("update group failed: %v", err)
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/v1/pages/%s/component-groups/%s", pageID, groupID))
	if err != nil {
		t.Fatalf("delete group failed: %v", err)
	}
}

// ============================================================================
// Subscriber CRUD Lifecycle
// ============================================================================

// TestStatuspageIntegrationSubscriberCRUDLifecycle tests full CRUD lifecycle.
func TestStatuspageIntegrationSubscriberCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pageID := "integ-page-sub"

	var sub map[string]interface{}
	err := c.Post(ctx, fmt.Sprintf("/v1/pages/%s/subscribers", pageID), spBody(t, map[string]interface{}{
		"subscriber": map[string]interface{}{
			"email":         "integ@example.com",
			"endpoint":      "https://hooks.example.com/sp",
			"component_ids": []string{"comp-a"},
		},
	}), &sub)
	if err != nil {
		t.Fatalf("create subscriber failed: %v", err)
	}
	subID, _ := sub["id"].(string)

	// Read
	var readSub map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s/subscribers/%s", pageID, subID), &readSub)
	if err != nil {
		t.Fatalf("read subscriber failed: %v", err)
	}

	// Update
	var updatedSub map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s/subscribers/%s", pageID, subID), spBody(t, map[string]interface{}{
		"subscriber": map[string]interface{}{"email": "updated@example.com"},
	}), &updatedSub)
	if err != nil {
		t.Fatalf("update subscriber failed: %v", err)
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/v1/pages/%s/subscribers/%s", pageID, subID))
	if err != nil {
		t.Fatalf("delete subscriber failed: %v", err)
	}
}

// ============================================================================
// Incident Template CRUD Lifecycle
// ============================================================================

// TestStatuspageIntegrationIncidentTemplateCRUDLifecycle tests full CRUD lifecycle.
func TestStatuspageIntegrationIncidentTemplateCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pageID := "integ-page-it"

	var tmpl map[string]interface{}
	err := c.Post(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates", pageID), spBody(t, map[string]interface{}{
		"template": map[string]interface{}{
			"name":  "Outage",
			"title": "Service Outage",
			"body":  "Investigating.",
		},
	}), &tmpl)
	if err != nil {
		t.Fatalf("create incident template failed: %v", err)
	}
	tmplID, _ := tmpl["id"].(string)

	// Read
	var readTmpl map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates/%s", pageID, tmplID), &readTmpl)
	if err != nil {
		t.Fatalf("read incident template failed: %v", err)
	}

	// Update
	var updatedTmpl map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates/%s", pageID, tmplID), spBody(t, map[string]interface{}{
		"template": map[string]interface{}{"title": "Major Outage"},
	}), &updatedTmpl)
	if err != nil {
		t.Fatalf("update incident template failed: %v", err)
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates/%s", pageID, tmplID))
	if err != nil {
		t.Fatalf("delete incident template failed: %v", err)
	}
}

// ============================================================================
// Maintenance Template CRUD Lifecycle
// ============================================================================

// TestStatuspageIntegrationMaintenanceTemplateCRUDLifecycle tests full CRUD lifecycle.
func TestStatuspageIntegrationMaintenanceTemplateCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pageID := "integ-page-mt"

	var tmpl map[string]interface{}
	err := c.Post(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates", pageID), spBody(t, map[string]interface{}{
		"template": map[string]interface{}{
			"name":  "Weekly Maintenance",
			"title": "Scheduled Maintenance",
			"body":  "Maintenance window.",
		},
	}), &tmpl)
	if err != nil {
		t.Fatalf("create maintenance template failed: %v", err)
	}
	tmplID, _ := tmpl["id"].(string)

	// Read
	var readTmpl map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", pageID, tmplID), &readTmpl)
	if err != nil {
		t.Fatalf("read maintenance template failed: %v", err)
	}

	// Update
	var updatedTmpl map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", pageID, tmplID), spBody(t, map[string]interface{}{
		"template": map[string]interface{}{"body": "Extended maintenance."},
	}), &updatedTmpl)
	if err != nil {
		t.Fatalf("update maintenance template failed: %v", err)
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", pageID, tmplID))
	if err != nil {
		t.Fatalf("delete maintenance template failed: %v", err)
	}
}

// ============================================================================
// Permission CRUD Lifecycle
// ============================================================================

// TestStatuspageIntegrationPermissionCRUDLifecycle tests full CRUD lifecycle.
func TestStatuspageIntegrationPermissionCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pageID := "integ-page-perm"

	var perm map[string]interface{}
	err := c.Post(ctx, fmt.Sprintf("/v1/pages/%s/permissions", pageID), spBody(t, map[string]interface{}{
		"permission": map[string]interface{}{
			"principal_type": "user",
			"principal_id":   "user-456",
			"role":           "member",
		},
	}), &perm)
	if err != nil {
		t.Fatalf("create permission failed: %v", err)
	}
	permID, _ := perm["id"].(string)

	// Read
	var readPerm map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s/permissions/%s", pageID, permID), &readPerm)
	if err != nil {
		t.Fatalf("read permission failed: %v", err)
	}
	if readPerm["role"] != "member" {
		t.Errorf("expected role 'member', got %v", readPerm["role"])
	}

	// Update role
	var updatedPerm map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s/permissions/%s", pageID, permID), spBody(t, map[string]interface{}{
		"permission": map[string]interface{}{"role": "admin"},
	}), &updatedPerm)
	if err != nil {
		t.Fatalf("update permission failed: %v", err)
	}
	if updatedPerm["role"] != "admin" {
		t.Errorf("expected role 'admin', got %v", updatedPerm["role"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/v1/pages/%s/permissions/%s", pageID, permID))
	if err != nil {
		t.Fatalf("delete permission failed: %v", err)
	}
}

// ============================================================================
// Cross-Resource Integration
// ============================================================================

// TestStatuspageIntegrationCrossResource tests cross-resource operations.
func TestStatuspageIntegrationCrossResource(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create page
	var page map[string]interface{}
	err := c.Post(ctx, "/v1/pages", spBody(t, map[string]interface{}{
		"page": map[string]interface{}{"name": "Cross Resource Page", "subdomain": "crossres"},
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID, _ := page["id"].(string)

	// Create component group
	var group map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/v1/pages/%s/component-groups", pageID), spBody(t, map[string]interface{}{
		"component_group": map[string]interface{}{"name": "Core Services"},
	}), &group)
	if err != nil {
		t.Fatalf("create component group failed: %v", err)
	}

	// Create component
	var comp map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/v1/pages/%s/components", pageID), spBody(t, map[string]interface{}{
		"component": map[string]interface{}{"name": "API", "status": "operational"},
	}), &comp)
	if err != nil {
		t.Fatalf("create component failed: %v", err)
	}
	compID, _ := comp["id"].(string)

	// Create subscriber with component
	var sub map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/v1/pages/%s/subscribers", pageID), spBody(t, map[string]interface{}{
		"subscriber": map[string]interface{}{
			"email":         "cross@example.com",
			"component_ids": []string{compID},
		},
	}), &sub)
	if err != nil {
		t.Fatalf("create subscriber failed: %v", err)
	}

	// Create incident template
	var it map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/v1/pages/%s/incident_templates", pageID), spBody(t, map[string]interface{}{
		"template": map[string]interface{}{"name": "Cross IT", "title": "T", "body": "B"},
	}), &it)
	if err != nil {
		t.Fatalf("create incident template failed: %v", err)
	}

	// Create maintenance template
	var mt map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/v1/pages/%s/maintenance_templates", pageID), spBody(t, map[string]interface{}{
		"template": map[string]interface{}{"name": "Cross MT", "title": "T", "body": "B"},
	}), &mt)
	if err != nil {
		t.Fatalf("create maintenance template failed: %v", err)
	}

	// Create permission
	var perm map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/v1/pages/%s/permissions", pageID), spBody(t, map[string]interface{}{
		"permission": map[string]interface{}{
			"principal_type": "group",
			"principal_id":   "grp-999",
			"role":           "viewer",
		},
	}), &perm)
	if err != nil {
		t.Fatalf("create permission failed: %v", err)
	}

	// Verify all can be read
	subID, _ := sub["id"].(string)
	itID, _ := it["id"].(string)
	mtID, _ := mt["id"].(string)
	permID, _ := perm["id"].(string)
	groupID, _ := group["id"].(string)

	var tmp map[string]interface{}
	for _, path := range []string{
		fmt.Sprintf("/v1/pages/%s", pageID),
		fmt.Sprintf("/v1/pages/%s/components/%s", pageID, compID),
		fmt.Sprintf("/v1/pages/%s/component-groups/%s", pageID, groupID),
		fmt.Sprintf("/v1/pages/%s/subscribers/%s", pageID, subID),
		fmt.Sprintf("/v1/pages/%s/incident_templates/%s", pageID, itID),
		fmt.Sprintf("/v1/pages/%s/maintenance_templates/%s", pageID, mtID),
		fmt.Sprintf("/v1/pages/%s/permissions/%s", pageID, permID),
	} {
		err = c.Get(ctx, path, &tmp)
		if err != nil {
			t.Errorf("read %s failed: %v", path, err)
		}
	}
}

// ============================================================================
// Drift Detection
// ============================================================================

// TestStatuspageIntegrationDriftDetection tests drift detection for a page.
func TestStatuspageIntegrationDriftDetection(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create page
	var page map[string]interface{}
	err := c.Post(ctx, "/v1/pages", spBody(t, map[string]interface{}{
		"page": map[string]interface{}{"name": "Drift Page", "subdomain": "drift"},
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID, _ := page["id"].(string)

	// Read original
	var original map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s", pageID), &original)
	if err != nil {
		t.Fatalf("read page failed: %v", err)
	}

	// Simulate external drift by updating directly
	var drifted map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s", pageID), spBody(t, map[string]interface{}{
		"page": map[string]interface{}{"name": "Drifted Page"},
	}), &drifted)
	if err != nil {
		t.Fatalf("drift update failed: %v", err)
	}

	// Re-read and detect drift
	var reread map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s", pageID), &reread)
	if err != nil {
		t.Fatalf("re-read page failed: %v", err)
	}
	if reread["name"] != "Drifted Page" {
		t.Errorf("drift not detected: expected 'Drifted Page', got %v", reread["name"])
	}
	if reread["name"] == original["name"] {
		t.Error("expected name to have changed from original")
	}
}

// ============================================================================
// Error Handling
// ============================================================================

// TestStatuspageIntegrationNotFoundErrors tests 404 error handling.
func TestStatuspageIntegrationNotFoundErrors(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	paths := []string{
		"/v1/pages/nonexistent",
		"/v1/pages/p1/components/nonexistent",
		"/v1/pages/p1/component-groups/nonexistent",
		"/v1/pages/p1/subscribers/nonexistent",
		"/v1/pages/p1/incident_templates/nonexistent",
		"/v1/pages/p1/maintenance_templates/nonexistent",
		"/v1/pages/p1/permissions/nonexistent",
	}

	for _, path := range paths {
		var result map[string]interface{}
		err := c.Get(ctx, path, &result)
		if err == nil {
			t.Errorf("expected error for GET %s", path)
		}
	}
}

// TestStatuspageIntegrationIdempotency tests that double apply yields no changes.
func TestStatuspageIntegrationIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupStatuspageMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create page
	body := map[string]interface{}{
		"page": map[string]interface{}{"name": "Idempotent Page", "subdomain": "idempotent"},
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/v1/pages", spBody(t, body), &created)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID, _ := created["id"].(string)

	// Update with same values
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/v1/pages/%s", pageID), spBody(t, body), &updated)
	if err != nil {
		t.Fatalf("idempotent update failed: %v", err)
	}

	// Read and verify unchanged
	var reread map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/v1/pages/%s", pageID), &reread)
	if err != nil {
		t.Fatalf("re-read failed: %v", err)
	}
	if reread["name"] != "Idempotent Page" {
		t.Errorf("expected name unchanged, got %v", reread["name"])
	}
}
