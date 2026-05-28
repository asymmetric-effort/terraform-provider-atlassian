// Package unit contains unit tests for the mock API Statuspage endpoints.
package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// newStatuspageServer creates a mock server with Statuspage endpoints registered.
func newStatuspageServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterStatuspageEndpoints(s)
	return httptest.NewServer(s.Handler())
}

// TestSPPageCRUDLifecycle tests create, read, update, and delete for pages.
func TestSPPageCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	// Create page
	resp := postJSON(t, ts.URL+"/v1/pages", map[string]interface{}{
		"page": map[string]interface{}{
			"name":             "My Status Page",
			"page_description": "Desc",
			"subdomain":        "mystatus",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create page: expected 201, got %d", resp.StatusCode)
	}
	var page map[string]interface{}
	decodeJSON(t, resp, &page)
	if page["name"] != "My Status Page" {
		t.Errorf("expected name 'My Status Page', got %v", page["name"])
	}
	id, _ := page["id"].(string)
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if page["url"] != "https://mystatus.statuspage.io" {
		t.Errorf("expected url 'https://mystatus.statuspage.io', got %v", page["url"])
	}

	// Read page
	readResp, err := http.Get(ts.URL + "/v1/pages/" + id)
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read page: expected 200, got %d", readResp.StatusCode)
	}
	var readPage map[string]interface{}
	decodeJSON(t, readResp, &readPage)
	if readPage["name"] != "My Status Page" {
		t.Errorf("read page: expected name, got %v", readPage["name"])
	}

	// Update page
	resp = putJSON(t, ts.URL+"/v1/pages/"+id, map[string]interface{}{
		"page": map[string]interface{}{
			"name": "Updated Page",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update page: expected 200, got %d", resp.StatusCode)
	}
	var updatedPage map[string]interface{}
	decodeJSON(t, resp, &updatedPage)
	if updatedPage["name"] != "Updated Page" {
		t.Errorf("update: expected name 'Updated Page', got %v", updatedPage["name"])
	}

	// Delete page
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/pages/"+id, nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete page: %v", err)
	}
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete page: expected 204, got %d", delResp.StatusCode)
	}

	// Verify deleted
	readResp, err = http.Get(ts.URL + "/v1/pages/" + id)
	if err != nil {
		t.Fatalf("read after delete: %v", err)
	}
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 after delete, got %d", readResp.StatusCode)
	}
}

// TestSPComponentCRUDLifecycle tests component CRUD.
func TestSPComponentCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	pageID := "test-page-1"

	// Create component
	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/components", map[string]interface{}{
		"component": map[string]interface{}{
			"name":        "API",
			"description": "API Component",
			"status":      "operational",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create component: expected 201, got %d", resp.StatusCode)
	}
	var comp map[string]interface{}
	decodeJSON(t, resp, &comp)
	id, _ := comp["id"].(string)
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	if comp["page_id"] != pageID {
		t.Errorf("expected page_id %q, got %v", pageID, comp["page_id"])
	}
	if comp["status"] != "operational" {
		t.Errorf("expected status 'operational', got %v", comp["status"])
	}

	// Read
	readResp, err := http.Get(ts.URL + "/v1/pages/" + pageID + "/components/" + id)
	if err != nil {
		t.Fatalf("read component: %v", err)
	}
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read component: expected 200, got %d", readResp.StatusCode)
	}

	// Update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/components/"+id, map[string]interface{}{
		"component": map[string]interface{}{
			"name":   "Updated API",
			"status": "degraded_performance",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update component: expected 200, got %d", resp.StatusCode)
	}
	var updated map[string]interface{}
	decodeJSON(t, resp, &updated)
	if updated["name"] != "Updated API" {
		t.Errorf("expected name 'Updated API', got %v", updated["name"])
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/pages/"+pageID+"/components/"+id, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete component: expected 204, got %d", delResp.StatusCode)
	}
}

// TestSPComponentGroupCRUDLifecycle tests component group CRUD.
func TestSPComponentGroupCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	pageID := "test-page-2"

	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups", map[string]interface{}{
		"component_group": map[string]interface{}{
			"name":        "Infrastructure",
			"description": "Infra group",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create group: expected 201, got %d", resp.StatusCode)
	}
	var group map[string]interface{}
	decodeJSON(t, resp, &group)
	id, _ := group["id"].(string)

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/component-groups/" + id)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read group: expected 200, got %d", readResp.StatusCode)
	}

	// Update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups/"+id, map[string]interface{}{
		"component_group": map[string]interface{}{
			"name": "Updated Infra",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update group: expected 200, got %d", resp.StatusCode)
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/pages/"+pageID+"/component-groups/"+id, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete group: expected 204, got %d", delResp.StatusCode)
	}
}

// TestSPSubscriberCRUDLifecycle tests subscriber CRUD.
func TestSPSubscriberCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	pageID := "test-page-3"

	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers", map[string]interface{}{
		"subscriber": map[string]interface{}{
			"email":         "user@example.com",
			"component_ids": []string{"comp-1", "comp-2"},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create subscriber: expected 201, got %d", resp.StatusCode)
	}
	var sub map[string]interface{}
	decodeJSON(t, resp, &sub)
	id, _ := sub["id"].(string)
	if sub["email"] != "user@example.com" {
		t.Errorf("expected email, got %v", sub["email"])
	}

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/subscribers/" + id)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read subscriber: expected 200, got %d", readResp.StatusCode)
	}

	// Update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers/"+id, map[string]interface{}{
		"subscriber": map[string]interface{}{
			"email": "updated@example.com",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update subscriber: expected 200, got %d", resp.StatusCode)
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/pages/"+pageID+"/subscribers/"+id, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete subscriber: expected 204, got %d", delResp.StatusCode)
	}
}

// TestSPIncidentTemplateCRUDLifecycle tests incident template CRUD.
func TestSPIncidentTemplateCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	pageID := "test-page-4"

	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates", map[string]interface{}{
		"template": map[string]interface{}{
			"name":  "Outage Template",
			"title": "Service Outage",
			"body":  "We are investigating an outage.",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create incident template: expected 201, got %d", resp.StatusCode)
	}
	var tmpl map[string]interface{}
	decodeJSON(t, resp, &tmpl)
	id, _ := tmpl["id"].(string)
	if tmpl["name"] != "Outage Template" {
		t.Errorf("expected name 'Outage Template', got %v", tmpl["name"])
	}

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/incident_templates/" + id)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read incident template: expected 200, got %d", readResp.StatusCode)
	}

	// Update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates/"+id, map[string]interface{}{
		"template": map[string]interface{}{
			"name": "Updated Outage",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update incident template: expected 200, got %d", resp.StatusCode)
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/pages/"+pageID+"/incident_templates/"+id, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete incident template: expected 204, got %d", delResp.StatusCode)
	}
}

// TestSPMaintenanceTemplateCRUDLifecycle tests maintenance template CRUD.
func TestSPMaintenanceTemplateCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	pageID := "test-page-5"

	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates", map[string]interface{}{
		"template": map[string]interface{}{
			"name":  "Maintenance Template",
			"title": "Scheduled Maintenance",
			"body":  "Maintenance window.",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create maintenance template: expected 201, got %d", resp.StatusCode)
	}
	var tmpl map[string]interface{}
	decodeJSON(t, resp, &tmpl)
	id, _ := tmpl["id"].(string)

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/maintenance_templates/" + id)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read maintenance template: expected 200, got %d", readResp.StatusCode)
	}

	// Update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/"+id, map[string]interface{}{
		"template": map[string]interface{}{
			"name": "Updated Maintenance",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update maintenance template: expected 200, got %d", resp.StatusCode)
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/"+id, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete maintenance template: expected 204, got %d", delResp.StatusCode)
	}
}

// TestSPPermissionCRUDLifecycle tests permission CRUD.
func TestSPPermissionCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	pageID := "test-page-6"

	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions", map[string]interface{}{
		"permission": map[string]interface{}{
			"principal_type": "user",
			"principal_id":   "user-123",
			"role":           "admin",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create permission: expected 201, got %d", resp.StatusCode)
	}
	var perm map[string]interface{}
	decodeJSON(t, resp, &perm)
	id, _ := perm["id"].(string)
	if perm["role"] != "admin" {
		t.Errorf("expected role 'admin', got %v", perm["role"])
	}

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/permissions/" + id)
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("read permission: expected 200, got %d", readResp.StatusCode)
	}

	// Update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions/"+id, map[string]interface{}{
		"permission": map[string]interface{}{
			"role": "viewer",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update permission: expected 200, got %d", resp.StatusCode)
	}
	var updated map[string]interface{}
	decodeJSON(t, resp, &updated)
	if updated["role"] != "viewer" {
		t.Errorf("expected role 'viewer', got %v", updated["role"])
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/v1/pages/"+pageID+"/permissions/"+id, nil)
	delResp, _ := http.DefaultClient.Do(req)
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete permission: expected 204, got %d", delResp.StatusCode)
	}
}

// TestSPPageNotFound tests 404 responses for nonexistent page.
func TestSPPageNotFound(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/v1/pages/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestSPComponentNotFound tests 404 responses for nonexistent component.
func TestSPComponentNotFound(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/v1/pages/p1/components/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestSPPageListEmpty tests listing pages when empty.
func TestSPPageListEmpty(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/v1/pages")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list pages: expected 200, got %d", resp.StatusCode)
	}
	var pages []json.RawMessage
	decodeJSON(t, resp, &pages)
	if len(pages) != 0 {
		t.Errorf("expected 0 pages, got %d", len(pages))
	}
}

// TestSPComponentListEmpty tests listing components when empty.
func TestSPComponentListEmpty(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/v1/pages/p1/components")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list components: expected 200, got %d", resp.StatusCode)
	}
}

// TestSPPageCreateMissingName tests error when creating page without name.
func TestSPPageCreateMissingName(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/pages", map[string]interface{}{
		"page": map[string]interface{}{},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestSPComponentCreateMissingName tests error when creating component without name.
func TestSPComponentCreateMissingName(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/pages/p1/components", map[string]interface{}{
		"component": map[string]interface{}{},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestSPPermissionCreateMissingFields tests error when creating permission without required fields.
func TestSPPermissionCreateMissingFields(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/v1/pages/p1/permissions", map[string]interface{}{
		"permission": map[string]interface{}{},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestSPDeleteNonexistentResources tests 404 on deleting nonexistent resources.
func TestSPDeleteNonexistentResources(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

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
		req, _ := http.NewRequest(http.MethodDelete, ts.URL+path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("delete %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("delete %s: expected 404, got %d", path, resp.StatusCode)
		}
	}
}

// TestSPUpdateNonexistentResources tests 404 on updating nonexistent resources.
func TestSPUpdateNonexistentResources(t *testing.T) {
	t.Parallel()
	ts := newStatuspageServer(t)
	defer ts.Close()

	testCases := []struct {
		path string
		body map[string]interface{}
	}{
		{"/v1/pages/nonexistent", map[string]interface{}{"page": map[string]interface{}{"name": "x"}}},
		{"/v1/pages/p1/components/nonexistent", map[string]interface{}{"component": map[string]interface{}{"name": "x"}}},
		{"/v1/pages/p1/component-groups/nonexistent", map[string]interface{}{"component_group": map[string]interface{}{"name": "x"}}},
		{"/v1/pages/p1/subscribers/nonexistent", map[string]interface{}{"subscriber": map[string]interface{}{"email": "x"}}},
		{"/v1/pages/p1/incident_templates/nonexistent", map[string]interface{}{"template": map[string]interface{}{"name": "x"}}},
		{"/v1/pages/p1/maintenance_templates/nonexistent", map[string]interface{}{"template": map[string]interface{}{"name": "x"}}},
		{"/v1/pages/p1/permissions/nonexistent", map[string]interface{}{"permission": map[string]interface{}{"role": "admin"}}},
	}
	for _, tc := range testCases {
		resp := putJSON(t, ts.URL+tc.path, tc.body)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("update %s: expected 404, got %d", tc.path, resp.StatusCode)
		}
	}
}
