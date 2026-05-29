// Package integration contains integration tests for Confluence resources.
//
// These tests exercise the internal/client package against the mock API server,
// verifying full CRUD lifecycles, cross-resource operations, idempotency,
// drift detection, import patterns, and error handling for all Confluence
// resource types: space, page, template, space_permission, content_restriction.
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

// setupConfluenceMockServer creates a mock server with auth, identity, and
// Confluence endpoints, and returns the httptest server and a configured client.
func setupConfluenceMockServer(t *testing.T) (*httptest.Server, *client.Client) {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterConfluenceEndpoints(s)
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

// confluenceBody marshals v to a bytes.Reader for use as a request body.
func confluenceBody(t *testing.T, v interface{}) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}
	return bytes.NewReader(data)
}

// ============================================================================
// Space CRUD Lifecycle
// ============================================================================

func TestConfluenceIntegrationSpaceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space
	createBody := map[string]interface{}{
		"name": "Engineering Space",
		"key":  "ENG",
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatal("create space: expected non-empty id")
	}
	if created["key"] != "ENG" {
		t.Errorf("create space: expected key 'ENG', got %v", created["key"])
	}
	if created["name"] != "Engineering Space" {
		t.Errorf("create space: expected name 'Engineering Space', got %v", created["name"])
	}
	if created["type"] != "global" {
		t.Errorf("create space: expected type 'global', got %v", created["type"])
	}

	// Read by ID
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), &read)
	if err != nil {
		t.Fatalf("read space by id failed: %v", err)
	}
	if read["id"] != id {
		t.Errorf("read: expected id %q, got %v", id, read["id"])
	}
	if read["key"] != "ENG" {
		t.Errorf("read: expected key 'ENG', got %v", read["key"])
	}

	// Read by key (lookup via list endpoint)
	var listResp map[string]interface{}
	err = c.Get(ctx, "/wiki/api/v2/spaces?key=ENG", &listResp)
	if err != nil {
		t.Fatalf("list spaces by key failed: %v", err)
	}
	results, ok := listResp["results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("list by key: expected 1 result, got %v", len(results))
	}

	// Update
	updateBody := map[string]interface{}{
		"name": "Engineering Hub",
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), confluenceBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update space failed: %v", err)
	}
	if updated["name"] != "Engineering Hub" {
		t.Errorf("update: expected name 'Engineering Hub', got %v", updated["name"])
	}
	if updated["id"] != id {
		t.Errorf("update: id should not change")
	}

	// Re-read to verify persistence
	var reread map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), &reread)
	if err != nil {
		t.Fatalf("re-read space failed: %v", err)
	}
	if reread["name"] != "Engineering Hub" {
		t.Errorf("re-read: update not persisted, expected 'Engineering Hub', got %v", reread["name"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id))
	if err != nil {
		t.Fatalf("delete space failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted space, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted space, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Page CRUD Lifecycle
// ============================================================================

func TestConfluenceIntegrationPageCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// First create a space to host the page
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Page Test Space",
		"key":  "PTS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	// Create page
	createBody := map[string]interface{}{
		"title":   "Getting Started",
		"spaceId": spaceID,
		"body": map[string]interface{}{
			"representation": "storage",
			"value":          "<p>Welcome to the space.</p>",
		},
	}
	var created map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID, ok := created["id"].(string)
	if !ok || pageID == "" {
		t.Fatal("create page: expected non-empty id")
	}
	if created["title"] != "Getting Started" {
		t.Errorf("create page: expected title 'Getting Started', got %v", created["title"])
	}
	if created["spaceId"] != spaceID {
		t.Errorf("create page: expected spaceId %q, got %v", spaceID, created["spaceId"])
	}
	if created["status"] != "current" {
		t.Errorf("create page: expected status 'current', got %v", created["status"])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &read)
	if err != nil {
		t.Fatalf("read page failed: %v", err)
	}
	if read["id"] != pageID {
		t.Errorf("read: expected id %q, got %v", pageID, read["id"])
	}
	if read["title"] != "Getting Started" {
		t.Errorf("read: expected title 'Getting Started', got %v", read["title"])
	}

	// Update
	updateBody := map[string]interface{}{
		"title": "Quick Start Guide",
		"body": map[string]interface{}{
			"representation": "storage",
			"value":          "<p>Updated content.</p>",
		},
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), confluenceBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update page failed: %v", err)
	}
	if updated["title"] != "Quick Start Guide" {
		t.Errorf("update: expected title 'Quick Start Guide', got %v", updated["title"])
	}
	if updated["id"] != pageID {
		t.Errorf("update: id should not change")
	}

	// Re-read to verify persistence
	var reread map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &reread)
	if err != nil {
		t.Fatalf("re-read page failed: %v", err)
	}
	if reread["title"] != "Quick Start Guide" {
		t.Errorf("re-read: update not persisted, expected 'Quick Start Guide', got %v", reread["title"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID))
	if err != nil {
		t.Fatalf("delete page failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted page, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted page, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Template CRUD Lifecycle
// ============================================================================

func TestConfluenceIntegrationTemplateCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:     "Template",
		basePath: "/wiki/api/v2/templates",
		idField:  "id",
		createBody: map[string]interface{}{
			"name":        "Meeting Notes",
			"description": "Template for meeting notes",
			"body":        "<h1>Meeting Notes</h1><p>Date: </p>",
		},
		updateBody: map[string]interface{}{
			"name": "Updated Meeting Notes",
		},
		verifyField:  "name",
		verifyCreate: "Meeting Notes",
		verifyUpdate: "Updated Meeting Notes",
	})
}

// ============================================================================
// Space Permission CRUD Lifecycle
// ============================================================================

func TestConfluenceIntegrationSpacePermissionCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a space first
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Permission Test Space",
		"key":  "PERM",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	// Create permission
	createBody := map[string]interface{}{
		"principalType": "user",
		"principalId":   "user-123",
		"operation":     "read",
	}
	var created map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), confluenceBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create permission failed: %v", err)
	}
	permID, ok := created["id"].(string)
	if !ok || permID == "" {
		t.Fatal("create permission: expected non-empty id")
	}
	if created["spaceId"] != spaceID {
		t.Errorf("create permission: expected spaceId %q, got %v", spaceID, created["spaceId"])
	}
	if created["principalType"] != "user" {
		t.Errorf("create permission: expected principalType 'user', got %v", created["principalType"])
	}

	// List permissions for space
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &listResp)
	if err != nil {
		t.Fatalf("list permissions failed: %v", err)
	}
	results, ok := listResp["results"].([]interface{})
	if !ok || len(results) < 1 {
		t.Fatalf("list permissions: expected at least 1 result, got %v", results)
	}

	// Delete permission
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions/%s", spaceID, permID))
	if err != nil {
		t.Fatalf("delete permission failed: %v", err)
	}

	// Verify empty list after delete
	var listAfter map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &listAfter)
	if err != nil {
		t.Fatalf("list permissions after delete failed: %v", err)
	}
	resultsAfter, ok := listAfter["results"].([]interface{})
	if !ok {
		t.Fatal("list after delete: expected results array")
	}
	if len(resultsAfter) != 0 {
		t.Errorf("list after delete: expected 0 results, got %d", len(resultsAfter))
	}
}

// ============================================================================
// Content Restriction CRUD Lifecycle
// ============================================================================

func TestConfluenceIntegrationContentRestrictionCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a space and page first
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Restriction Test Space",
		"key":  "RTS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var page map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Restricted Page",
		"spaceId": spaceID,
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := page["id"].(string)

	// Create restriction
	createBody := map[string]interface{}{
		"operation": "read",
		"restrictions": map[string]interface{}{
			"user": []map[string]interface{}{
				{"accountId": "user-abc"},
			},
		},
	}
	var created map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), confluenceBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create restriction failed: %v", err)
	}
	restrictionID, ok := created["id"].(string)
	if !ok || restrictionID == "" {
		t.Fatal("create restriction: expected non-empty id")
	}
	if created["contentId"] != pageID {
		t.Errorf("create restriction: expected contentId %q, got %v", pageID, created["contentId"])
	}
	if created["operation"] != "read" {
		t.Errorf("create restriction: expected operation 'read', got %v", created["operation"])
	}

	// List restrictions for page
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), &listResp)
	if err != nil {
		t.Fatalf("list restrictions failed: %v", err)
	}
	results, ok := listResp["results"].([]interface{})
	if !ok || len(results) < 1 {
		t.Fatalf("list restrictions: expected at least 1 result, got %v", results)
	}

	// Delete restriction
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions/%s", pageID, restrictionID))
	if err != nil {
		t.Fatalf("delete restriction failed: %v", err)
	}

	// Verify empty list after delete
	var listAfter map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), &listAfter)
	if err != nil {
		t.Fatalf("list restrictions after delete failed: %v", err)
	}
	resultsAfter, ok := listAfter["results"].([]interface{})
	if !ok {
		t.Fatal("list after delete: expected results array")
	}
	if len(resultsAfter) != 0 {
		t.Errorf("list after delete: expected 0 results, got %d", len(resultsAfter))
	}
}

// ============================================================================
// Cross-Resource: Space -> Page -> Permission -> Restriction
// ============================================================================

func TestConfluenceIntegrationCrossResourceFullWorkflow(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Step 1: Create space
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Cross-Resource Space",
		"key":  "XRES",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)
	t.Logf("created space: %s (key=XRES)", spaceID)

	// Step 2: Add page to space
	var page map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Architecture Overview",
		"spaceId": spaceID,
		"body": map[string]interface{}{
			"representation": "storage",
			"value":          "<p>System architecture document.</p>",
		},
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := page["id"].(string)
	t.Logf("created page: %s (title=Architecture Overview)", pageID)

	// Step 3: Set space permissions
	var perm map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), confluenceBody(t, map[string]interface{}{
		"principalType": "group",
		"principalId":   "engineering-team",
		"operation":     "read",
	}), &perm)
	if err != nil {
		t.Fatalf("create space permission failed: %v", err)
	}
	permID := perm["id"].(string)
	t.Logf("created permission: %s (group=engineering-team)", permID)

	// Step 4: Add content restriction on page
	var restriction map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), confluenceBody(t, map[string]interface{}{
		"operation": "update",
		"restrictions": map[string]interface{}{
			"group": []map[string]interface{}{
				{"name": "architects"},
			},
		},
	}), &restriction)
	if err != nil {
		t.Fatalf("create content restriction failed: %v", err)
	}
	restrictionID := restriction["id"].(string)
	t.Logf("created restriction: %s (operation=update)", restrictionID)

	// Verify all resources are readable
	var readSpace map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", spaceID), &readSpace)
	if err != nil {
		t.Errorf("cross-resource read space failed: %v", err)
	}
	if readSpace["key"] != "XRES" {
		t.Errorf("cross-resource space key mismatch: got %v", readSpace["key"])
	}

	var readPage map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &readPage)
	if err != nil {
		t.Errorf("cross-resource read page failed: %v", err)
	}
	if readPage["spaceId"] != spaceID {
		t.Errorf("cross-resource page spaceId mismatch: expected %q, got %v", spaceID, readPage["spaceId"])
	}

	var readPerms map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &readPerms)
	if err != nil {
		t.Errorf("cross-resource read permissions failed: %v", err)
	}
	permResults, _ := readPerms["results"].([]interface{})
	if len(permResults) < 1 {
		t.Errorf("cross-resource: expected at least 1 permission, got %d", len(permResults))
	}

	var readRestrictions map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), &readRestrictions)
	if err != nil {
		t.Errorf("cross-resource read restrictions failed: %v", err)
	}
	restrictionResults, _ := readRestrictions["results"].([]interface{})
	if len(restrictionResults) < 1 {
		t.Errorf("cross-resource: expected at least 1 restriction, got %d", len(restrictionResults))
	}

	// Verify pages belong to the correct space via list
	var pagesList map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages?space_id=%s", spaceID), &pagesList)
	if err != nil {
		t.Fatalf("list pages by space failed: %v", err)
	}
	pagesResults, _ := pagesList["results"].([]interface{})
	if len(pagesResults) != 1 {
		t.Errorf("expected 1 page in space, got %d", len(pagesResults))
	}

	// Cleanup in reverse dependency order
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions/%s", pageID, restrictionID))
	if err != nil {
		t.Errorf("cleanup delete restriction failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions/%s", spaceID, permID))
	if err != nil {
		t.Errorf("cleanup delete permission failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID))
	if err != nil {
		t.Errorf("cleanup delete page failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", spaceID))
	if err != nil {
		t.Errorf("cleanup delete space failed: %v", err)
	}
}

// ============================================================================
// Import (Read-by-ID after Create) Tests
// ============================================================================

func TestConfluenceIntegrationImportSpaceByID(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Import Space",
		"key":  "IMS",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["id"] != id {
		t.Errorf("import: id mismatch: %v vs %v", imported["id"], id)
	}
	if imported["name"] != "Import Space" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
	if imported["key"] != "IMS" {
		t.Errorf("import: key mismatch: got %v", imported["key"])
	}
}

func TestConfluenceIntegrationImportPageByID(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a space first
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Import Page Space",
		"key":  "IPS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var created map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Import Page",
		"spaceId": spaceID,
	}), &created)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["id"] != pageID {
		t.Errorf("import: id mismatch: %v vs %v", imported["id"], pageID)
	}
	if imported["title"] != "Import Page" {
		t.Errorf("import: title mismatch: got %v", imported["title"])
	}
	if imported["spaceId"] != spaceID {
		t.Errorf("import: spaceId mismatch: got %v", imported["spaceId"])
	}
}

func TestConfluenceIntegrationImportTemplateByID(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/templates", confluenceBody(t, map[string]interface{}{
		"name":        "Import Template",
		"description": "A template for import testing",
	}), &created)
	if err != nil {
		t.Fatalf("create template failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["id"] != id {
		t.Errorf("import: id mismatch: %v vs %v", imported["id"], id)
	}
	if imported["name"] != "Import Template" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestConfluenceIntegrationImportSpacePermissionByList(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Import Perm Space",
		"key":  "IMPR",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	// Create permission
	var created map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), confluenceBody(t, map[string]interface{}{
		"principalType": "user",
		"principalId":   "import-user",
		"operation":     "write",
	}), &created)
	if err != nil {
		t.Fatalf("create permission failed: %v", err)
	}
	permID := created["id"].(string)

	// Import by listing permissions for the space
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &listResp)
	if err != nil {
		t.Fatalf("import list failed: %v", err)
	}
	results, ok := listResp["results"].([]interface{})
	if !ok || len(results) < 1 {
		t.Fatal("import: expected at least 1 permission")
	}
	found := false
	for _, r := range results {
		rm, _ := r.(map[string]interface{})
		if rm["id"] == permID {
			found = true
			if rm["principalType"] != "user" {
				t.Errorf("import: principalType mismatch: got %v", rm["principalType"])
			}
		}
	}
	if !found {
		t.Errorf("import: permission %s not found in list", permID)
	}
}

func TestConfluenceIntegrationImportContentRestrictionByList(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space and page
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Import Restriction Space",
		"key":  "IMRS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var page map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Import Restriction Page",
		"spaceId": spaceID,
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := page["id"].(string)

	// Create restriction
	var created map[string]interface{}
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), confluenceBody(t, map[string]interface{}{
		"operation": "read",
	}), &created)
	if err != nil {
		t.Fatalf("create restriction failed: %v", err)
	}
	restrictionID := created["id"].(string)

	// Import by listing restrictions for the content
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), &listResp)
	if err != nil {
		t.Fatalf("import list failed: %v", err)
	}
	results, ok := listResp["results"].([]interface{})
	if !ok || len(results) < 1 {
		t.Fatal("import: expected at least 1 restriction")
	}
	found := false
	for _, r := range results {
		rm, _ := r.(map[string]interface{})
		if rm["id"] == restrictionID {
			found = true
			if rm["operation"] != "read" {
				t.Errorf("import: operation mismatch: got %v", rm["operation"])
			}
		}
	}
	if !found {
		t.Errorf("import: restriction %s not found in list", restrictionID)
	}
}

// ============================================================================
// Idempotency Tests
// ============================================================================

func TestConfluenceIntegrationSpaceUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Idempotent Space",
		"key":  "IDMS",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	updateBody := map[string]interface{}{"name": "Idempotent Space"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), confluenceBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), confluenceBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["name"] != second["name"] {
		t.Errorf("idempotency: name differs: %v vs %v", first["name"], second["name"])
	}
	if first["key"] != second["key"] {
		t.Errorf("idempotency: key differs: %v vs %v", first["key"], second["key"])
	}
	if first["id"] != second["id"] {
		t.Errorf("idempotency: id differs: %v vs %v", first["id"], second["id"])
	}
}

func TestConfluenceIntegrationPageUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Page Idempotency Space",
		"key":  "PIDS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var created map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Idempotent Page",
		"spaceId": spaceID,
	}), &created)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := created["id"].(string)

	updateBody := map[string]interface{}{"title": "Idempotent Page"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), confluenceBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), confluenceBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["title"] != second["title"] {
		t.Errorf("idempotency: title differs: %v vs %v", first["title"], second["title"])
	}
	if first["id"] != second["id"] {
		t.Errorf("idempotency: id differs: %v vs %v", first["id"], second["id"])
	}
	if first["spaceId"] != second["spaceId"] {
		t.Errorf("idempotency: spaceId differs: %v vs %v", first["spaceId"], second["spaceId"])
	}
}

func TestConfluenceIntegrationTemplateUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/templates", confluenceBody(t, map[string]interface{}{
		"name":        "Idempotent Template",
		"description": "Same description",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	updateBody := map[string]interface{}{"name": "Idempotent Template", "description": "Same description"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", id), confluenceBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", id), confluenceBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["name"] != second["name"] {
		t.Errorf("idempotency: name differs: %v vs %v", first["name"], second["name"])
	}
	if first["description"] != second["description"] {
		t.Errorf("idempotency: description differs: %v vs %v", first["description"], second["description"])
	}
	if first["id"] != second["id"] {
		t.Errorf("idempotency: id differs: %v vs %v", first["id"], second["id"])
	}
}

// ============================================================================
// Drift Detection Tests
// ============================================================================

func TestConfluenceIntegrationDriftDetectionSpaceModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Drift Space",
		"key":  "DRFS",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// Simulate external modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id),
		confluenceBody(t, map[string]interface{}{"name": "Externally Renamed Space"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read should reflect external change (drift detection)
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["name"] != "Externally Renamed Space" {
		t.Errorf("drift not detected: expected 'Externally Renamed Space', got %v", current["name"])
	}
}

func TestConfluenceIntegrationDriftDetectionPageModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Drift Page Space",
		"key":  "DRFP",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var created map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Drift Page",
		"spaceId": spaceID,
	}), &created)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := created["id"].(string)

	// External modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID),
		confluenceBody(t, map[string]interface{}{"title": "Externally Renamed Page"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read should reflect external change
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["title"] != "Externally Renamed Page" {
		t.Errorf("drift not detected: expected 'Externally Renamed Page', got %v", current["title"])
	}
}

func TestConfluenceIntegrationDriftDetectionTemplateModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/templates", confluenceBody(t, map[string]interface{}{
		"name":        "Drift Template",
		"description": "Original",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", id),
		confluenceBody(t, map[string]interface{}{"description": "Externally Updated"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read should reflect change
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", id), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["description"] != "Externally Updated" {
		t.Errorf("drift not detected: expected 'Externally Updated', got %v", current["description"])
	}
}

func TestConfluenceIntegrationDriftDetectionSpaceDeletedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Ephemeral Space",
		"key":  "EPHS",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External delete
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id))
	if err != nil {
		t.Fatalf("external delete failed: %v", err)
	}

	// Drift detection: read should return 404
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), &ghost)
	if err == nil {
		t.Fatal("expected error for externally deleted space, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestConfluenceIntegrationDriftDetectionPageDeletedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Drift Delete Page Space",
		"key":  "DDPS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var created map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Ephemeral Page",
		"spaceId": spaceID,
	}), &created)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := created["id"].(string)

	// External delete
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID))
	if err != nil {
		t.Fatalf("external delete failed: %v", err)
	}

	// Drift detection: read should return 404
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &ghost)
	if err == nil {
		t.Fatal("expected error for externally deleted page, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Error Handling: Missing Fields
// ============================================================================

func TestConfluenceIntegrationMissingRequiredFields(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tests := []struct {
		name string
		path string
		body map[string]interface{}
	}{
		{"Space missing name", "/wiki/api/v2/spaces", map[string]interface{}{"key": "NONAME"}},
		{"Space missing key", "/wiki/api/v2/spaces", map[string]interface{}{"name": "No Key Space"}},
		{"Page missing title", "/wiki/api/v2/pages", map[string]interface{}{"spaceId": "some-space"}},
		{"Page missing spaceId", "/wiki/api/v2/pages", map[string]interface{}{"title": "No Space Page"}},
		{"Template missing name", "/wiki/api/v2/templates", map[string]interface{}{"description": "no name"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Post(ctx, tt.path, confluenceBody(t, tt.body), nil)
			if err == nil {
				t.Fatalf("[%s] expected error for missing required fields, got nil", tt.name)
			}
			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("[%s] expected *client.APIError, got %T", tt.name, err)
			}
			if apiErr.StatusCode != 400 {
				t.Errorf("[%s] expected 400, got %d", tt.name, apiErr.StatusCode)
			}
		})
	}
}

func TestConfluenceIntegrationPermissionMissingPrincipalType(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space first
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Missing Principal Space",
		"key":  "MPS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	// Missing principalType
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID),
		confluenceBody(t, map[string]interface{}{"operation": "read"}), nil)
	if err == nil {
		t.Fatal("expected error for missing principalType, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

func TestConfluenceIntegrationRestrictionMissingOperation(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space and page
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Missing Op Space",
		"key":  "MOS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var page map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Missing Op Page",
		"spaceId": spaceID,
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := page["id"].(string)

	// Missing operation
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID),
		confluenceBody(t, map[string]interface{}{"restrictions": map[string]interface{}{}}), nil)
	if err == nil {
		t.Fatal("expected error for missing operation, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Error Handling: Not Found
// ============================================================================

func TestConfluenceIntegrationReadNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	paths := []struct {
		name string
		path string
	}{
		{"Space", "/wiki/api/v2/spaces/nonexistent"},
		{"Page", "/wiki/api/v2/pages/nonexistent"},
		{"Template", "/wiki/api/v2/templates/nonexistent"},
	}

	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			var result map[string]interface{}
			err := c.Get(ctx, p.path, &result)
			if err == nil {
				t.Fatalf("[%s] expected error for nonexistent resource", p.name)
			}
			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("[%s] expected *client.APIError, got %T", p.name, err)
			}
			if apiErr.StatusCode != 404 {
				t.Errorf("[%s] expected 404, got %d", p.name, apiErr.StatusCode)
			}
		})
	}
}

func TestConfluenceIntegrationDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	paths := []struct {
		name string
		path string
	}{
		{"Space", "/wiki/api/v2/spaces/nonexistent"},
		{"Page", "/wiki/api/v2/pages/nonexistent"},
		{"Template", "/wiki/api/v2/templates/nonexistent"},
		{"Permission", "/wiki/api/v2/spaces/nonexistent/permissions/nonexistent"},
		{"Restriction", "/wiki/api/v2/content/nonexistent/restrictions/nonexistent"},
	}

	for _, p := range paths {
		t.Run(p.name, func(t *testing.T) {
			err := c.Delete(ctx, p.path)
			if err == nil {
				t.Fatalf("[%s] expected error for nonexistent resource", p.name)
			}
			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("[%s] expected *client.APIError, got %T", p.name, err)
			}
			if apiErr.StatusCode != 404 {
				t.Errorf("[%s] expected 404, got %d", p.name, apiErr.StatusCode)
			}
		})
	}
}

// ============================================================================
// Error Handling: Duplicates
// ============================================================================

func TestConfluenceIntegrationSpaceDuplicateKey(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]interface{}{
		"name": "First Space",
		"key":  "DUPS",
	}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, body), nil)
	if err != nil {
		t.Fatalf("create first space failed: %v", err)
	}

	// Duplicate key should fail with 409
	body2 := map[string]interface{}{
		"name": "Second Space",
		"key":  "DUPS",
	}
	err = c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, body2), nil)
	if err == nil {
		t.Fatal("expected error for duplicate space key, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected 409, got %d", apiErr.StatusCode)
	}
}

func TestConfluenceIntegrationTemplateDuplicateName(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]interface{}{
		"name": "Duplicate Template",
	}
	err := c.Post(ctx, "/wiki/api/v2/templates", confluenceBody(t, body), nil)
	if err != nil {
		t.Fatalf("create first template failed: %v", err)
	}

	// Duplicate name should fail with 409
	err = c.Post(ctx, "/wiki/api/v2/templates", confluenceBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for duplicate template name, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected 409, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// State Consistency Tests
// ============================================================================

func TestConfluenceIntegrationStateConsistencyCreateReadMatch(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Space: create response should match subsequent read
	var createdSpace map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "State Space",
		"key":  "STSP",
	}), &createdSpace)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := createdSpace["id"].(string)

	var readSpace map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", spaceID), &readSpace)
	if err != nil {
		t.Fatalf("read space failed: %v", err)
	}
	if createdSpace["name"] != readSpace["name"] {
		t.Errorf("state mismatch: create name %v != read name %v", createdSpace["name"], readSpace["name"])
	}
	if createdSpace["key"] != readSpace["key"] {
		t.Errorf("state mismatch: create key %v != read key %v", createdSpace["key"], readSpace["key"])
	}
	if createdSpace["id"] != readSpace["id"] {
		t.Errorf("state mismatch: create id %v != read id %v", createdSpace["id"], readSpace["id"])
	}

	// Page: create response should match subsequent read
	var createdPage map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "State Page",
		"spaceId": spaceID,
	}), &createdPage)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := createdPage["id"].(string)

	var readPage map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageID), &readPage)
	if err != nil {
		t.Fatalf("read page failed: %v", err)
	}
	if createdPage["title"] != readPage["title"] {
		t.Errorf("state mismatch: create title %v != read title %v", createdPage["title"], readPage["title"])
	}
	if createdPage["spaceId"] != readPage["spaceId"] {
		t.Errorf("state mismatch: create spaceId %v != read spaceId %v", createdPage["spaceId"], readPage["spaceId"])
	}
	if createdPage["id"] != readPage["id"] {
		t.Errorf("state mismatch: create id %v != read id %v", createdPage["id"], readPage["id"])
	}

	// Template: create response should match subsequent read
	var createdTemplate map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/templates", confluenceBody(t, map[string]interface{}{
		"name": "State Template",
	}), &createdTemplate)
	if err != nil {
		t.Fatalf("create template failed: %v", err)
	}
	templateID := createdTemplate["id"].(string)

	var readTemplate map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/templates/%s", templateID), &readTemplate)
	if err != nil {
		t.Fatalf("read template failed: %v", err)
	}
	if createdTemplate["name"] != readTemplate["name"] {
		t.Errorf("state mismatch: create name %v != read name %v", createdTemplate["name"], readTemplate["name"])
	}
	if createdTemplate["id"] != readTemplate["id"] {
		t.Errorf("state mismatch: create id %v != read id %v", createdTemplate["id"], readTemplate["id"])
	}
}

func TestConfluenceIntegrationStateConsistencyUpdateReadMatch(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space, update, verify update response matches read
	var created map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Consistency Space",
		"key":  "CNSS",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id),
		confluenceBody(t, map[string]interface{}{"name": "Updated Consistency Space"}), &updated)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s", id), &read)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if updated["name"] != read["name"] {
		t.Errorf("state mismatch: update name %v != read name %v", updated["name"], read["name"])
	}
	if updated["id"] != read["id"] {
		t.Errorf("state mismatch: update id %v != read id %v", updated["id"], read["id"])
	}
}

// ============================================================================
// List Endpoint Tests
// ============================================================================

func TestConfluenceIntegrationListSpaces(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create two spaces
	for _, key := range []string{"LSA", "LSB"} {
		err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
			"name": "List Space " + key,
			"key":  key,
		}), nil)
		if err != nil {
			t.Fatalf("create space %s failed: %v", key, err)
		}
	}

	// List all spaces
	var listResp map[string]interface{}
	err := c.Get(ctx, "/wiki/api/v2/spaces", &listResp)
	if err != nil {
		t.Fatalf("list spaces failed: %v", err)
	}
	results, ok := listResp["results"].([]interface{})
	if !ok {
		t.Fatal("expected results array in list response")
	}
	if len(results) < 2 {
		t.Errorf("expected at least 2 spaces, got %d", len(results))
	}

	// Filter by key
	var filtered map[string]interface{}
	err = c.Get(ctx, "/wiki/api/v2/spaces?key=LSA", &filtered)
	if err != nil {
		t.Fatalf("list spaces by key failed: %v", err)
	}
	filteredResults, _ := filtered["results"].([]interface{})
	if len(filteredResults) != 1 {
		t.Errorf("expected 1 space for key=LSA, got %d", len(filteredResults))
	}
}

func TestConfluenceIntegrationListPagesBySpace(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create two spaces with pages in each
	var space1, space2 map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Pages Space 1",
		"key":  "PS1",
	}), &space1)
	if err != nil {
		t.Fatalf("create space 1 failed: %v", err)
	}
	space1ID := space1["id"].(string)

	err = c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Pages Space 2",
		"key":  "PS2",
	}), &space2)
	if err != nil {
		t.Fatalf("create space 2 failed: %v", err)
	}
	space2ID := space2["id"].(string)

	// Create 2 pages in space1, 1 page in space2
	for _, title := range []string{"Page A", "Page B"} {
		err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
			"title":   title,
			"spaceId": space1ID,
		}), nil)
		if err != nil {
			t.Fatalf("create page %s failed: %v", title, err)
		}
	}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Page C",
		"spaceId": space2ID,
	}), nil)
	if err != nil {
		t.Fatalf("create page C failed: %v", err)
	}

	// List pages for space1 only
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages?space_id=%s", space1ID), &listResp)
	if err != nil {
		t.Fatalf("list pages by space failed: %v", err)
	}
	results, _ := listResp["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("expected 2 pages in space1, got %d", len(results))
	}

	// List pages for space2
	var listResp2 map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages?space_id=%s", space2ID), &listResp2)
	if err != nil {
		t.Fatalf("list pages for space2 failed: %v", err)
	}
	results2, _ := listResp2["results"].([]interface{})
	if len(results2) != 1 {
		t.Errorf("expected 1 page in space2, got %d", len(results2))
	}
}

func TestConfluenceIntegrationListPermissionsBySpace(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create two spaces
	var space1, space2 map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Perm List Space 1",
		"key":  "PL1",
	}), &space1)
	if err != nil {
		t.Fatalf("create space 1 failed: %v", err)
	}
	space1ID := space1["id"].(string)

	err = c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Perm List Space 2",
		"key":  "PL2",
	}), &space2)
	if err != nil {
		t.Fatalf("create space 2 failed: %v", err)
	}
	space2ID := space2["id"].(string)

	// Add permission to space1 only
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", space1ID),
		confluenceBody(t, map[string]interface{}{
			"principalType": "user",
			"principalId":   "list-user",
			"operation":     "read",
		}), nil)
	if err != nil {
		t.Fatalf("create permission failed: %v", err)
	}

	// Space1 should have 1 permission
	var list1 map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", space1ID), &list1)
	if err != nil {
		t.Fatalf("list permissions for space1 failed: %v", err)
	}
	results1, _ := list1["results"].([]interface{})
	if len(results1) != 1 {
		t.Errorf("expected 1 permission for space1, got %d", len(results1))
	}

	// Space2 should have 0 permissions
	var list2 map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", space2ID), &list2)
	if err != nil {
		t.Fatalf("list permissions for space2 failed: %v", err)
	}
	results2, _ := list2["results"].([]interface{})
	if len(results2) != 0 {
		t.Errorf("expected 0 permissions for space2, got %d", len(results2))
	}
}

func TestConfluenceIntegrationListRestrictionsByContent(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space and two pages
	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Restriction List Space",
		"key":  "RLS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var page1, page2 map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Restricted Page 1",
		"spaceId": spaceID,
	}), &page1)
	if err != nil {
		t.Fatalf("create page 1 failed: %v", err)
	}
	page1ID := page1["id"].(string)

	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Unrestricted Page 2",
		"spaceId": spaceID,
	}), &page2)
	if err != nil {
		t.Fatalf("create page 2 failed: %v", err)
	}
	page2ID := page2["id"].(string)

	// Add restriction to page1 only
	err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", page1ID),
		confluenceBody(t, map[string]interface{}{
			"operation": "read",
		}), nil)
	if err != nil {
		t.Fatalf("create restriction failed: %v", err)
	}

	// Page1 should have 1 restriction
	var list1 map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", page1ID), &list1)
	if err != nil {
		t.Fatalf("list restrictions for page1 failed: %v", err)
	}
	results1, _ := list1["results"].([]interface{})
	if len(results1) != 1 {
		t.Errorf("expected 1 restriction for page1, got %d", len(results1))
	}

	// Page2 should have 0 restrictions
	var list2 map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", page2ID), &list2)
	if err != nil {
		t.Fatalf("list restrictions for page2 failed: %v", err)
	}
	results2, _ := list2["results"].([]interface{})
	if len(results2) != 0 {
		t.Errorf("expected 0 restrictions for page2, got %d", len(results2))
	}
}

// ============================================================================
// Multiple Pages in Space
// ============================================================================

func TestConfluenceIntegrationMultiplePagesInSpace(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Multi Page Space",
		"key":  "MPGS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	// Create 5 pages
	pageIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		var pg map[string]interface{}
		err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
			"title":   fmt.Sprintf("Page %d", i+1),
			"spaceId": spaceID,
		}), &pg)
		if err != nil {
			t.Fatalf("create page %d failed: %v", i+1, err)
		}
		pageIDs[i] = pg["id"].(string)
	}

	// List should show all 5
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages?space_id=%s", spaceID), &listResp)
	if err != nil {
		t.Fatalf("list pages failed: %v", err)
	}
	results, _ := listResp["results"].([]interface{})
	if len(results) != 5 {
		t.Errorf("expected 5 pages, got %d", len(results))
	}

	// Delete one, list should show 4
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/pages/%s", pageIDs[2]))
	if err != nil {
		t.Fatalf("delete page failed: %v", err)
	}

	var listAfter map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/pages?space_id=%s", spaceID), &listAfter)
	if err != nil {
		t.Fatalf("list pages after delete failed: %v", err)
	}
	resultsAfter, _ := listAfter["results"].([]interface{})
	if len(resultsAfter) != 4 {
		t.Errorf("expected 4 pages after delete, got %d", len(resultsAfter))
	}
}

// ============================================================================
// Multiple Permissions on Space
// ============================================================================

func TestConfluenceIntegrationMultiplePermissionsOnSpace(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Multi Perm Space",
		"key":  "MPRM",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	// Create 3 permissions
	permIDs := make([]string, 3)
	for i, pt := range []string{"user", "group", "user"} {
		var perm map[string]interface{}
		err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID),
			confluenceBody(t, map[string]interface{}{
				"principalType": pt,
				"principalId":   fmt.Sprintf("principal-%d", i),
				"operation":     "read",
			}), &perm)
		if err != nil {
			t.Fatalf("create permission %d failed: %v", i, err)
		}
		permIDs[i] = perm["id"].(string)
	}

	// List should show 3
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &listResp)
	if err != nil {
		t.Fatalf("list permissions failed: %v", err)
	}
	results, _ := listResp["results"].([]interface{})
	if len(results) != 3 {
		t.Errorf("expected 3 permissions, got %d", len(results))
	}

	// Delete one, list should show 2
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions/%s", spaceID, permIDs[1]))
	if err != nil {
		t.Fatalf("delete permission failed: %v", err)
	}

	var listAfter map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/spaces/%s/permissions", spaceID), &listAfter)
	if err != nil {
		t.Fatalf("list permissions after delete failed: %v", err)
	}
	resultsAfter, _ := listAfter["results"].([]interface{})
	if len(resultsAfter) != 2 {
		t.Errorf("expected 2 permissions after delete, got %d", len(resultsAfter))
	}
}

// ============================================================================
// Multiple Restrictions on Page
// ============================================================================

func TestConfluenceIntegrationMultipleRestrictionsOnPage(t *testing.T) {
	t.Parallel()
	_, c := setupConfluenceMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var space map[string]interface{}
	err := c.Post(ctx, "/wiki/api/v2/spaces", confluenceBody(t, map[string]interface{}{
		"name": "Multi Restriction Space",
		"key":  "MRS",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)

	var page map[string]interface{}
	err = c.Post(ctx, "/wiki/api/v2/pages", confluenceBody(t, map[string]interface{}{
		"title":   "Multi Restriction Page",
		"spaceId": spaceID,
	}), &page)
	if err != nil {
		t.Fatalf("create page failed: %v", err)
	}
	pageID := page["id"].(string)

	// Create read and update restrictions
	restrictionIDs := make([]string, 2)
	for i, op := range []string{"read", "update"} {
		var restriction map[string]interface{}
		err = c.Post(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID),
			confluenceBody(t, map[string]interface{}{
				"operation": op,
			}), &restriction)
		if err != nil {
			t.Fatalf("create restriction %d failed: %v", i, err)
		}
		restrictionIDs[i] = restriction["id"].(string)
	}

	// List should show 2
	var listResp map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), &listResp)
	if err != nil {
		t.Fatalf("list restrictions failed: %v", err)
	}
	results, _ := listResp["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("expected 2 restrictions, got %d", len(results))
	}

	// Delete one, list should show 1
	err = c.Delete(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions/%s", pageID, restrictionIDs[0]))
	if err != nil {
		t.Fatalf("delete restriction failed: %v", err)
	}

	var listAfter map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/wiki/api/v2/content/%s/restrictions", pageID), &listAfter)
	if err != nil {
		t.Fatalf("list restrictions after delete failed: %v", err)
	}
	resultsAfter, _ := listAfter["results"].([]interface{})
	if len(resultsAfter) != 1 {
		t.Errorf("expected 1 restriction after delete, got %d", len(resultsAfter))
	}
}
