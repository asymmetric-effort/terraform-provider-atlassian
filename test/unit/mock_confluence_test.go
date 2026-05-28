// Package unit contains unit tests for the mock API Confluence endpoints.
package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// newConfluenceServer creates a mock server with Confluence endpoints registered.
func newConfluenceServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterConfluenceEndpoints(s)
	return httptest.NewServer(s.Handler())
}

// TestConfluenceSpaceCRUDLifecycle tests create, read, update, list, and delete for Confluence spaces.
func TestConfluenceSpaceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	// Create space
	resp := postJSON(t, ts.URL+"/wiki/api/v2/spaces", map[string]string{
		"name": "Engineering",
		"key":  "ENG",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create space: expected 201, got %d", resp.StatusCode)
	}
	var space map[string]interface{}
	decodeJSON(t, resp, &space)
	spaceID, ok := space["id"].(string)
	if !ok || spaceID == "" {
		t.Fatal("create space: expected non-empty id")
	}
	if space["key"] != "ENG" {
		t.Errorf("create space: expected key 'ENG', got %v", space["key"])
	}
	if space["name"] != "Engineering" {
		t.Errorf("create space: expected name 'Engineering', got %v", space["name"])
	}

	// Read space by ID
	resp, err := http.Get(ts.URL + "/wiki/api/v2/spaces/" + spaceID)
	if err != nil {
		t.Fatalf("read space: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read space: expected 200, got %d", resp.StatusCode)
	}
	var readSpace map[string]interface{}
	decodeJSON(t, resp, &readSpace)
	if readSpace["id"] != spaceID {
		t.Errorf("read space: expected id %q, got %v", spaceID, readSpace["id"])
	}

	// Read space by key
	resp, err = http.Get(ts.URL + "/wiki/api/v2/spaces/ENG")
	if err != nil {
		t.Fatalf("read space by key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read space by key: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update space
	resp = putJSON(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID, map[string]string{
		"name": "Engineering Updated",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update space: expected 200, got %d", resp.StatusCode)
	}
	var updatedSpace map[string]interface{}
	decodeJSON(t, resp, &updatedSpace)
	if updatedSpace["name"] != "Engineering Updated" {
		t.Errorf("update space: expected name 'Engineering Updated', got %v", updatedSpace["name"])
	}
	if updatedSpace["id"] != spaceID {
		t.Errorf("update space: id should not change")
	}

	// List spaces
	resp, err = http.Get(ts.URL + "/wiki/api/v2/spaces")
	if err != nil {
		t.Fatalf("list spaces: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list spaces: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	results, ok := listResp["results"].([]interface{})
	if !ok {
		t.Fatal("list spaces: expected results array")
	}
	if len(results) != 1 {
		t.Errorf("list spaces: expected 1 space, got %d", len(results))
	}

	// List spaces by key query param
	resp, err = http.Get(ts.URL + "/wiki/api/v2/spaces?key=ENG")
	if err != nil {
		t.Fatalf("list spaces by key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list spaces by key: expected 200, got %d", resp.StatusCode)
	}
	var keyListResp map[string]interface{}
	decodeJSON(t, resp, &keyListResp)
	keyResults, ok := keyListResp["results"].([]interface{})
	if !ok || len(keyResults) != 1 {
		t.Errorf("list spaces by key: expected 1 result, got %v", keyListResp["results"])
	}

	// List spaces by non-matching key
	resp, err = http.Get(ts.URL + "/wiki/api/v2/spaces?key=NONEXIST")
	if err != nil {
		t.Fatalf("list spaces by non-matching key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list spaces by non-matching key: expected 200, got %d", resp.StatusCode)
	}
	var emptyListResp map[string]interface{}
	decodeJSON(t, resp, &emptyListResp)
	emptyResults, ok := emptyListResp["results"].([]interface{})
	if !ok || len(emptyResults) != 0 {
		t.Errorf("list spaces by non-matching key: expected 0 results, got %v", emptyListResp["results"])
	}

	// Duplicate key
	resp = postJSON(t, ts.URL+"/wiki/api/v2/spaces", map[string]string{
		"name": "Another Space",
		"key":  "ENG",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate key: expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing fields
	resp = postJSON(t, ts.URL+"/wiki/api/v2/spaces", map[string]string{
		"name": "No Key",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing key: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete space
	resp = doDelete(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete space: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp, err = http.Get(ts.URL + "/wiki/api/v2/spaces/" + spaceID)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestConfluenceSpaceNotFound tests reading and deleting non-existent spaces.
func TestConfluenceSpaceNotFound(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/wiki/api/v2/spaces/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("read: expected 404, got %d", resp.StatusCode)
	}

	resp = doDelete(t, ts.URL+"/wiki/api/v2/spaces/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestConfluenceSpaceUpdateNotFound tests updating a non-existent space.
func TestConfluenceSpaceUpdateNotFound(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/wiki/api/v2/spaces/nonexistent", map[string]string{"name": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestConfluenceSpaceCreateInvalidJSON tests that invalid JSON in space create returns 400.
func TestConfluenceSpaceCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/wiki/api/v2/spaces", strings.NewReader("bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestConfluenceSpaceUpdateInvalidJSON tests that invalid JSON in space update returns 400.
func TestConfluenceSpaceUpdateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	// Create space first
	resp := postJSON(t, ts.URL+"/wiki/api/v2/spaces", map[string]string{
		"name": "Test", "key": "TST",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var space map[string]interface{}
	decodeJSON(t, resp, &space)
	id := space["id"].(string)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/wiki/api/v2/spaces/"+id, strings.NewReader("{bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", putResp.StatusCode)
	}
}

// TestConfluencePageCRUDLifecycle tests create, read, update, list, and delete for pages.
func TestConfluencePageCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	// Create page
	resp := postJSON(t, ts.URL+"/wiki/api/v2/pages", map[string]interface{}{
		"title":   "Getting Started",
		"spaceId": "space-1",
		"body": map[string]interface{}{
			"representation": "storage",
			"value":          "<p>Hello world</p>",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create page: expected 201, got %d", resp.StatusCode)
	}
	var page map[string]interface{}
	decodeJSON(t, resp, &page)
	pageID, ok := page["id"].(string)
	if !ok || pageID == "" {
		t.Fatal("create page: expected non-empty id")
	}
	if page["title"] != "Getting Started" {
		t.Errorf("create page: expected title 'Getting Started', got %v", page["title"])
	}
	if page["spaceId"] != "space-1" {
		t.Errorf("create page: expected spaceId 'space-1', got %v", page["spaceId"])
	}
	if page["status"] != "current" {
		t.Errorf("create page: expected status 'current', got %v", page["status"])
	}

	// Read page
	resp, err := http.Get(ts.URL + "/wiki/api/v2/pages/" + pageID)
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read page: expected 200, got %d", resp.StatusCode)
	}
	var readPage map[string]interface{}
	decodeJSON(t, resp, &readPage)
	if readPage["id"] != pageID {
		t.Errorf("read page: expected id %q, got %v", pageID, readPage["id"])
	}

	// Update page
	resp = putJSON(t, ts.URL+"/wiki/api/v2/pages/"+pageID, map[string]string{
		"title": "Getting Started (Updated)",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update page: expected 200, got %d", resp.StatusCode)
	}
	var updatedPage map[string]interface{}
	decodeJSON(t, resp, &updatedPage)
	if updatedPage["title"] != "Getting Started (Updated)" {
		t.Errorf("update page: expected title 'Getting Started (Updated)', got %v", updatedPage["title"])
	}
	if updatedPage["id"] != pageID {
		t.Errorf("update page: id should not change")
	}

	// Create second page in different space
	resp = postJSON(t, ts.URL+"/wiki/api/v2/pages", map[string]string{
		"title":   "Other Page",
		"spaceId": "space-2",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create second page: expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List all pages
	resp, err = http.Get(ts.URL + "/wiki/api/v2/pages")
	if err != nil {
		t.Fatalf("list pages: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list pages: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	results, ok := listResp["results"].([]interface{})
	if !ok {
		t.Fatal("list pages: expected results array")
	}
	if len(results) != 2 {
		t.Errorf("list pages: expected 2 pages, got %d", len(results))
	}

	// List pages filtered by space_id
	resp, err = http.Get(ts.URL + "/wiki/api/v2/pages?space_id=space-1")
	if err != nil {
		t.Fatalf("list pages by space_id: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list pages by space_id: expected 200, got %d", resp.StatusCode)
	}
	var filteredResp map[string]interface{}
	decodeJSON(t, resp, &filteredResp)
	filteredResults, ok := filteredResp["results"].([]interface{})
	if !ok || len(filteredResults) != 1 {
		t.Errorf("list pages by space_id: expected 1 page, got %v", filteredResp["results"])
	}

	// Missing fields
	resp = postJSON(t, ts.URL+"/wiki/api/v2/pages", map[string]string{
		"title": "No Space",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing spaceId: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete page
	resp = doDelete(t, ts.URL+"/wiki/api/v2/pages/"+pageID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete page: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp, err = http.Get(ts.URL + "/wiki/api/v2/pages/" + pageID)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestConfluencePageNotFound tests reading and deleting non-existent pages.
func TestConfluencePageNotFound(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/wiki/api/v2/pages/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("read: expected 404, got %d", resp.StatusCode)
	}

	resp = doDelete(t, ts.URL+"/wiki/api/v2/pages/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestConfluencePageUpdateNotFound tests updating a non-existent page.
func TestConfluencePageUpdateNotFound(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/wiki/api/v2/pages/nonexistent", map[string]string{"title": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestConfluencePageCreateInvalidJSON tests that invalid JSON in page create returns 400.
func TestConfluencePageCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/wiki/api/v2/pages", strings.NewReader("bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestConfluencePageUpdateInvalidJSON tests that invalid JSON in page update returns 400.
func TestConfluencePageUpdateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/wiki/api/v2/pages", map[string]string{
		"title": "Test", "spaceId": "sp-1",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var page map[string]interface{}
	decodeJSON(t, resp, &page)
	id := page["id"].(string)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/wiki/api/v2/pages/"+id, strings.NewReader("{bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", putResp.StatusCode)
	}
}

// TestConfluenceTemplateCRUDLifecycle tests create, read, update, list, and delete for templates.
func TestConfluenceTemplateCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	// Create template
	resp := postJSON(t, ts.URL+"/wiki/api/v2/templates", map[string]interface{}{
		"name":        "Meeting Notes",
		"description": "Template for meeting notes",
		"body": map[string]string{
			"representation": "storage",
			"value":          "<h1>Meeting Notes</h1>",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create template: expected 201, got %d", resp.StatusCode)
	}
	var tmpl map[string]interface{}
	decodeJSON(t, resp, &tmpl)
	tmplID, ok := tmpl["id"].(string)
	if !ok || tmplID == "" {
		t.Fatal("create template: expected non-empty id")
	}
	if tmpl["name"] != "Meeting Notes" {
		t.Errorf("create template: expected name 'Meeting Notes', got %v", tmpl["name"])
	}

	// Read template
	resp, err := http.Get(ts.URL + "/wiki/api/v2/templates/" + tmplID)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read template: expected 200, got %d", resp.StatusCode)
	}
	var readTmpl map[string]interface{}
	decodeJSON(t, resp, &readTmpl)
	if readTmpl["id"] != tmplID {
		t.Errorf("read template: expected id %q, got %v", tmplID, readTmpl["id"])
	}

	// Update template
	resp = putJSON(t, ts.URL+"/wiki/api/v2/templates/"+tmplID, map[string]string{
		"description": "Updated template",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update template: expected 200, got %d", resp.StatusCode)
	}
	var updatedTmpl map[string]interface{}
	decodeJSON(t, resp, &updatedTmpl)
	if updatedTmpl["description"] != "Updated template" {
		t.Errorf("update template: expected description 'Updated template', got %v", updatedTmpl["description"])
	}

	// List templates
	resp, err = http.Get(ts.URL + "/wiki/api/v2/templates")
	if err != nil {
		t.Fatalf("list templates: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list templates: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("list templates: expected 1, got %v", listResp["values"])
	}

	// Duplicate name
	resp = postJSON(t, ts.URL+"/wiki/api/v2/templates", map[string]string{
		"name": "Meeting Notes",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate name: expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing name
	resp = postJSON(t, ts.URL+"/wiki/api/v2/templates", map[string]string{
		"description": "no name",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing name: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete template
	resp = doDelete(t, ts.URL+"/wiki/api/v2/templates/"+tmplID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete template: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp, err = http.Get(ts.URL + "/wiki/api/v2/templates/" + tmplID)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestConfluenceTemplateNotFound tests reading and deleting non-existent templates.
func TestConfluenceTemplateNotFound(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/wiki/api/v2/templates/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("read: expected 404, got %d", resp.StatusCode)
	}

	resp = doDelete(t, ts.URL+"/wiki/api/v2/templates/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestSpacePermissionLifecycle tests create, list, and delete for space permissions.
func TestSpacePermissionLifecycle(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	spaceID := "space-abc"

	// Create permission
	resp := postJSON(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID+"/permissions", map[string]interface{}{
		"principalType": "user",
		"principalId":   "user-123",
		"operation":     "read",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create permission: expected 201, got %d", resp.StatusCode)
	}
	var perm map[string]interface{}
	decodeJSON(t, resp, &perm)
	permID, ok := perm["id"].(string)
	if !ok || permID == "" {
		t.Fatal("create permission: expected non-empty id")
	}
	if perm["spaceId"] != spaceID {
		t.Errorf("create permission: expected spaceId %q, got %v", spaceID, perm["spaceId"])
	}
	if perm["principalType"] != "user" {
		t.Errorf("create permission: expected principalType 'user', got %v", perm["principalType"])
	}

	// Create second permission
	resp = postJSON(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID+"/permissions", map[string]interface{}{
		"principalType": "group",
		"principalId":   "group-456",
		"operation":     "write",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create second permission: expected 201, got %d", resp.StatusCode)
	}
	var perm2 map[string]interface{}
	decodeJSON(t, resp, &perm2)
	perm2ID := perm2["id"].(string)

	// List permissions for space
	resp, err := http.Get(ts.URL + "/wiki/api/v2/spaces/" + spaceID + "/permissions")
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list permissions: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	results, ok := listResp["results"].([]interface{})
	if !ok || len(results) != 2 {
		t.Errorf("list permissions: expected 2, got %v", listResp["results"])
	}

	// List permissions for different space (should be empty)
	resp, err = http.Get(ts.URL + "/wiki/api/v2/spaces/other-space/permissions")
	if err != nil {
		t.Fatalf("list permissions other space: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list permissions other space: expected 200, got %d", resp.StatusCode)
	}
	var emptyListResp map[string]interface{}
	decodeJSON(t, resp, &emptyListResp)
	emptyResults, ok := emptyListResp["results"].([]interface{})
	if !ok || len(emptyResults) != 0 {
		t.Errorf("list permissions other space: expected 0, got %v", emptyListResp["results"])
	}

	// Missing principalType
	resp = postJSON(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID+"/permissions", map[string]interface{}{
		"principalId": "user-789",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing principalType: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete first permission
	resp = doDelete(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID+"/permissions/"+permID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete permission: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete second permission
	resp = doDelete(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID+"/permissions/"+perm2ID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete second permission: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete non-existent permission
	resp = doDelete(t, ts.URL+"/wiki/api/v2/spaces/"+spaceID+"/permissions/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete non-existent: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestSpacePermissionCreateInvalidJSON tests that invalid JSON in permission create returns 400.
func TestSpacePermissionCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/wiki/api/v2/spaces/sp-1/permissions", strings.NewReader("bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestContentRestrictionLifecycle tests create, list, and delete for content restrictions.
func TestContentRestrictionLifecycle(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	contentID := "page-abc"

	// Create restriction
	resp := postJSON(t, ts.URL+"/wiki/api/v2/content/"+contentID+"/restrictions", map[string]interface{}{
		"operation": "read",
		"restrictions": map[string]interface{}{
			"user": []map[string]string{
				{"accountId": "user-123"},
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create restriction: expected 201, got %d", resp.StatusCode)
	}
	var restriction map[string]interface{}
	decodeJSON(t, resp, &restriction)
	restrictionID, ok := restriction["id"].(string)
	if !ok || restrictionID == "" {
		t.Fatal("create restriction: expected non-empty id")
	}
	if restriction["contentId"] != contentID {
		t.Errorf("create restriction: expected contentId %q, got %v", contentID, restriction["contentId"])
	}
	if restriction["operation"] != "read" {
		t.Errorf("create restriction: expected operation 'read', got %v", restriction["operation"])
	}

	// Create second restriction
	resp = postJSON(t, ts.URL+"/wiki/api/v2/content/"+contentID+"/restrictions", map[string]interface{}{
		"operation": "update",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create second restriction: expected 201, got %d", resp.StatusCode)
	}
	var restriction2 map[string]interface{}
	decodeJSON(t, resp, &restriction2)
	restriction2ID := restriction2["id"].(string)

	// List restrictions for content
	resp, err := http.Get(ts.URL + "/wiki/api/v2/content/" + contentID + "/restrictions")
	if err != nil {
		t.Fatalf("list restrictions: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list restrictions: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	results, ok := listResp["results"].([]interface{})
	if !ok || len(results) != 2 {
		t.Errorf("list restrictions: expected 2, got %v", listResp["results"])
	}

	// List restrictions for different content (should be empty)
	resp, err = http.Get(ts.URL + "/wiki/api/v2/content/other-page/restrictions")
	if err != nil {
		t.Fatalf("list restrictions other content: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list restrictions other content: expected 200, got %d", resp.StatusCode)
	}
	var emptyListResp map[string]interface{}
	decodeJSON(t, resp, &emptyListResp)
	emptyResults, ok := emptyListResp["results"].([]interface{})
	if !ok || len(emptyResults) != 0 {
		t.Errorf("list restrictions other content: expected 0, got %v", emptyListResp["results"])
	}

	// Missing operation
	resp = postJSON(t, ts.URL+"/wiki/api/v2/content/"+contentID+"/restrictions", map[string]interface{}{
		"restrictions": map[string]interface{}{},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing operation: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete first restriction
	resp = doDelete(t, ts.URL+"/wiki/api/v2/content/"+contentID+"/restrictions/"+restrictionID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete restriction: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete second restriction
	resp = doDelete(t, ts.URL+"/wiki/api/v2/content/"+contentID+"/restrictions/"+restriction2ID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete second restriction: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete non-existent restriction
	resp = doDelete(t, ts.URL+"/wiki/api/v2/content/"+contentID+"/restrictions/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete non-existent: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestContentRestrictionCreateInvalidJSON tests that invalid JSON in restriction create returns 400.
func TestContentRestrictionCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/wiki/api/v2/content/pg-1/restrictions", strings.NewReader("bad"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestConfluenceErrorResponseFormat verifies error responses follow Atlassian format.
func TestConfluenceErrorResponseFormat(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/wiki/api/v2/spaces/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var errResp map[string]interface{}
	decodeJSON(t, resp, &errResp)

	msgs, ok := errResp["errorMessages"].([]interface{})
	if !ok || len(msgs) == 0 {
		t.Fatal("expected non-empty errorMessages array")
	}

	errs, ok := errResp["errors"].(map[string]interface{})
	if !ok {
		t.Fatal("expected errors object")
	}
	_ = errs
}

// TestConfluenceEmptyListResponses tests that listing endpoints return empty arrays when no resources exist.
func TestConfluenceEmptyListResponses(t *testing.T) {
	t.Parallel()
	ts := newConfluenceServer(t)
	defer ts.Close()

	type listTest struct {
		path     string
		arrayKey string
	}

	tests := []listTest{
		{"/wiki/api/v2/spaces", "results"},
		{"/wiki/api/v2/pages", "results"},
		{"/wiki/api/v2/templates", "values"},
		{"/wiki/api/v2/spaces/sp-1/permissions", "results"},
		{"/wiki/api/v2/content/pg-1/restrictions", "results"},
	}

	for _, tt := range tests {
		resp, err := http.Get(ts.URL + tt.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tt.path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d", tt.path, resp.StatusCode)
			resp.Body.Close()
			continue
		}
		var listResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			t.Fatalf("GET %s: decode error: %v", tt.path, err)
		}
		resp.Body.Close()
		arr, ok := listResp[tt.arrayKey].([]interface{})
		if !ok {
			t.Errorf("GET %s: expected %s array", tt.path, tt.arrayKey)
			continue
		}
		if len(arr) != 0 {
			t.Errorf("GET %s: expected empty %s, got %d", tt.path, tt.arrayKey, len(arr))
		}
	}
}
