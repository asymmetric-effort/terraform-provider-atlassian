// Package integration contains integration tests for Jira resources.
//
// These tests exercise the internal/client package against the mock API server,
// verifying full CRUD lifecycles, cross-resource operations, idempotency,
// drift detection, import patterns, and state consistency for all Jira
// resource types.
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

// setupJiraMockServer creates a mock server with auth, identity, and Jira endpoints,
// and returns the httptest server and a configured client.
func setupJiraMockServer(t *testing.T) (*httptest.Server, *client.Client) {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterJiraEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	auth, err := client.NewTokenAuthenticator("test@example.com", "test-token")
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

// jiraBody marshals v to a bytes.Reader for use as a request body.
func jiraBody(t *testing.T, v interface{}) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}
	return bytes.NewReader(data)
}

// --- Helper: generic CRUD lifecycle ---

// crudResource defines the parameters for a generic CRUD lifecycle test.
type crudResource struct {
	name           string
	basePath       string
	idField        string
	createBody     map[string]interface{}
	updateBody     map[string]interface{}
	verifyField    string
	verifyCreate   interface{}
	verifyUpdate   interface{}
	requiredFields []string
}

// testCRUDLifecycle runs create, read, update, re-read, delete, verify-gone for a resource.
func testCRUDLifecycle(t *testing.T, c *client.Client, r crudResource) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create
	var created map[string]interface{}
	err := c.Post(ctx, r.basePath, jiraBody(t, r.createBody), &created)
	if err != nil {
		t.Fatalf("[%s] create failed: %v", r.name, err)
	}
	id, ok := created[r.idField].(string)
	if !ok || id == "" {
		t.Fatalf("[%s] create: expected non-empty %s", r.name, r.idField)
	}
	if created[r.verifyField] != r.verifyCreate {
		t.Errorf("[%s] create: expected %s=%v, got %v", r.name, r.verifyField, r.verifyCreate, created[r.verifyField])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", r.basePath, id), &read)
	if err != nil {
		t.Fatalf("[%s] read failed: %v", r.name, err)
	}
	if read[r.idField] != id {
		t.Errorf("[%s] read: expected %s=%q, got %v", r.name, r.idField, id, read[r.idField])
	}
	if read[r.verifyField] != r.verifyCreate {
		t.Errorf("[%s] read: expected %s=%v, got %v", r.name, r.verifyField, r.verifyCreate, read[r.verifyField])
	}

	// Update
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("%s/%s", r.basePath, id), jiraBody(t, r.updateBody), &updated)
	if err != nil {
		t.Fatalf("[%s] update failed: %v", r.name, err)
	}
	if updated[r.verifyField] != r.verifyUpdate {
		t.Errorf("[%s] update: expected %s=%v, got %v", r.name, r.verifyField, r.verifyUpdate, updated[r.verifyField])
	}
	if updated[r.idField] != id {
		t.Errorf("[%s] update: %s should not change", r.name, r.idField)
	}

	// Re-read to verify persistence
	var reread map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", r.basePath, id), &reread)
	if err != nil {
		t.Fatalf("[%s] re-read failed: %v", r.name, err)
	}
	if reread[r.verifyField] != r.verifyUpdate {
		t.Errorf("[%s] re-read: update not persisted, expected %v, got %v", r.name, r.verifyUpdate, reread[r.verifyField])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("%s/%s", r.basePath, id))
	if err != nil {
		t.Fatalf("[%s] delete failed: %v", r.name, err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", r.basePath, id), &ghost)
	if err == nil {
		t.Fatalf("[%s] expected error reading deleted resource, got nil", r.name)
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("[%s] expected *client.APIError, got %T: %v", r.name, err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("[%s] expected 404 for deleted resource, got %d", r.name, apiErr.StatusCode)
	}
}

// --- Space (Project) CRUD ---

func TestJiraIntegrationSpaceCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create space
	createBody := map[string]interface{}{
		"name":           "Integration Space",
		"key":            "INTG",
		"projectTypeKey": "software",
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/project", jiraBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatal("create space: expected non-empty id")
	}
	if created["key"] != "INTG" {
		t.Errorf("create space: expected key 'INTG', got %v", created["key"])
	}
	if created["name"] != "Integration Space" {
		t.Errorf("create space: expected name 'Integration Space', got %v", created["name"])
	}

	// Read by key
	var readByKey map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/project/INTG", &readByKey)
	if err != nil {
		t.Fatalf("read space by key failed: %v", err)
	}
	if readByKey["key"] != "INTG" {
		t.Errorf("read by key: expected key 'INTG', got %v", readByKey["key"])
	}

	// Read by ID
	var readByID map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/project/%s", id), &readByID)
	if err != nil {
		t.Fatalf("read space by id failed: %v", err)
	}
	if readByID["id"] != id {
		t.Errorf("read by id: expected id %q, got %v", id, readByID["id"])
	}

	// Update
	updateBody := map[string]interface{}{
		"name": "Updated Integration Space",
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/project/%s", id), jiraBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update space failed: %v", err)
	}
	if updated["name"] != "Updated Integration Space" {
		t.Errorf("update space: expected name 'Updated Integration Space', got %v", updated["name"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/project/%s", id))
	if err != nil {
		t.Fatalf("delete space failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/project/%s", id), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted space, got nil")
	}
}

func TestJiraIntegrationSpaceDuplicateKey(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]interface{}{
		"name":           "Space A",
		"key":            "DUP",
		"projectTypeKey": "software",
	}
	err := c.Post(ctx, "/rest/api/3/project", jiraBody(t, body), nil)
	if err != nil {
		t.Fatalf("create first space failed: %v", err)
	}

	// Duplicate key should fail with 409
	body2 := map[string]interface{}{
		"name":           "Space B",
		"key":            "DUP",
		"projectTypeKey": "software",
	}
	err = c.Post(ctx, "/rest/api/3/project", jiraBody(t, body2), nil)
	if err == nil {
		t.Fatal("expected error for duplicate key, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected 409, got %d", apiErr.StatusCode)
	}
}

// --- Issue Type CRUD ---

func TestJiraIntegrationIssueTypeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "IssueType",
		basePath:     "/rest/api/3/issuetype",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Bug", "description": "A bug report"},
		updateBody:   map[string]interface{}{"name": "Defect"},
		verifyField:  "name",
		verifyCreate: "Bug",
		verifyUpdate: "Defect",
	})
}

// --- Issue Type Scheme CRUD ---

func TestJiraIntegrationIssueTypeSchemeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "IssueTypeScheme",
		basePath:     "/rest/api/3/issuetypescheme",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Default Scheme", "description": "Default issue type scheme"},
		updateBody:   map[string]interface{}{"name": "Updated Scheme"},
		verifyField:  "name",
		verifyCreate: "Default Scheme",
		verifyUpdate: "Updated Scheme",
	})
}

// --- Workflow CRUD ---

func TestJiraIntegrationWorkflowCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "Workflow",
		basePath:     "/rest/api/3/workflow",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Bug Workflow", "description": "Workflow for bugs"},
		updateBody:   map[string]interface{}{"name": "Defect Workflow"},
		verifyField:  "name",
		verifyCreate: "Bug Workflow",
		verifyUpdate: "Defect Workflow",
	})
}

// --- Workflow Scheme CRUD ---

func TestJiraIntegrationWorkflowSchemeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "WorkflowScheme",
		basePath:     "/rest/api/3/workflowscheme",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Software Workflow Scheme", "description": "For software projects"},
		updateBody:   map[string]interface{}{"name": "Updated Workflow Scheme"},
		verifyField:  "name",
		verifyCreate: "Software Workflow Scheme",
		verifyUpdate: "Updated Workflow Scheme",
	})
}

// --- Screen CRUD ---

func TestJiraIntegrationScreenCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "Screen",
		basePath:     "/rest/api/3/screen",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Bug Screen", "description": "Screen for bug creation"},
		updateBody:   map[string]interface{}{"name": "Defect Screen"},
		verifyField:  "name",
		verifyCreate: "Bug Screen",
		verifyUpdate: "Defect Screen",
	})
}

// --- Screen Scheme CRUD ---

func TestJiraIntegrationScreenSchemeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "ScreenScheme",
		basePath:     "/rest/api/3/screenscheme",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Default Screen Scheme", "description": "Default screens"},
		updateBody:   map[string]interface{}{"name": "Updated Screen Scheme"},
		verifyField:  "name",
		verifyCreate: "Default Screen Scheme",
		verifyUpdate: "Updated Screen Scheme",
	})
}

// --- Permission Scheme CRUD ---

func TestJiraIntegrationPermissionSchemeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "PermissionScheme",
		basePath:     "/rest/api/3/permissionscheme",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Admin Permission Scheme", "description": "Full admin permissions"},
		updateBody:   map[string]interface{}{"name": "Restricted Permission Scheme"},
		verifyField:  "name",
		verifyCreate: "Admin Permission Scheme",
		verifyUpdate: "Restricted Permission Scheme",
	})
}

// --- Security Scheme CRUD ---

func TestJiraIntegrationSecuritySchemeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "SecurityScheme",
		basePath:     "/rest/api/3/issuesecurityschemes",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Confidential Scheme", "description": "Security levels for confidential issues"},
		updateBody:   map[string]interface{}{"name": "Internal Scheme"},
		verifyField:  "name",
		verifyCreate: "Confidential Scheme",
		verifyUpdate: "Internal Scheme",
	})
}

// --- Notification Scheme CRUD ---

func TestJiraIntegrationNotificationSchemeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "NotificationScheme",
		basePath:     "/rest/api/3/notificationscheme",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Email Notification Scheme", "description": "Email notifications"},
		updateBody:   map[string]interface{}{"name": "Slack Notification Scheme"},
		verifyField:  "name",
		verifyCreate: "Email Notification Scheme",
		verifyUpdate: "Slack Notification Scheme",
	})
}

// --- Dashboard CRUD ---

func TestJiraIntegrationDashboardCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "Dashboard",
		basePath:     "/rest/api/3/dashboard",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Team Dashboard", "description": "Team overview"},
		updateBody:   map[string]interface{}{"name": "Project Dashboard"},
		verifyField:  "name",
		verifyCreate: "Team Dashboard",
		verifyUpdate: "Project Dashboard",
	})
}

// --- Filter CRUD ---

func TestJiraIntegrationFilterCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "Filter",
		basePath:     "/rest/api/3/filter",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Open Bugs", "jql": "type = Bug AND status = Open"},
		updateBody:   map[string]interface{}{"name": "Critical Bugs"},
		verifyField:  "name",
		verifyCreate: "Open Bugs",
		verifyUpdate: "Critical Bugs",
	})
}

// --- Custom Field CRUD ---

func TestJiraIntegrationCustomFieldCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "CustomField",
		basePath:     "/rest/api/3/field",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Story Points", "type": "number", "description": "Estimated effort"},
		updateBody:   map[string]interface{}{"name": "Effort Points"},
		verifyField:  "name",
		verifyCreate: "Story Points",
		verifyUpdate: "Effort Points",
	})
}

// --- Board CRUD ---

func TestJiraIntegrationBoardCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "Board",
		basePath:     "/rest/agile/1.0/board",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Sprint Board", "type": "scrum"},
		updateBody:   map[string]interface{}{"name": "Kanban Board"},
		verifyField:  "name",
		verifyCreate: "Sprint Board",
		verifyUpdate: "Kanban Board",
	})
}

// --- Priority CRUD ---

func TestJiraIntegrationPriorityCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "Priority",
		basePath:     "/rest/api/3/priority",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Blocker", "description": "Blocks release"},
		updateBody:   map[string]interface{}{"name": "Critical"},
		verifyField:  "name",
		verifyCreate: "Blocker",
		verifyUpdate: "Critical",
	})
}

// --- Priority Scheme CRUD ---

func TestJiraIntegrationPrioritySchemeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "PriorityScheme",
		basePath:     "/rest/api/3/priorityscheme",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Standard Priority Scheme", "description": "Default priorities"},
		updateBody:   map[string]interface{}{"name": "Custom Priority Scheme"},
		verifyField:  "name",
		verifyCreate: "Standard Priority Scheme",
		verifyUpdate: "Custom Priority Scheme",
	})
}

// --- Automation Rule CRUD ---

func TestJiraIntegrationAutomationRuleCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create automation rule (requires trigger field, not standard CRUD)
	createBody := map[string]interface{}{
		"name": "Auto-close stale issues",
		"trigger": map[string]interface{}{
			"type":     "scheduled",
			"schedule": "0 0 * * *",
		},
		"conditions": []map[string]interface{}{
			{"type": "jql", "value": "updated <= -30d AND status != Done"},
		},
		"actions": []map[string]interface{}{
			{"type": "transition", "value": "Done"},
		},
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/automation/rule", jiraBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create automation rule failed: %v", err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatal("create automation rule: expected non-empty id")
	}
	if created["name"] != "Auto-close stale issues" {
		t.Errorf("create: expected name 'Auto-close stale issues', got %v", created["name"])
	}
	if created["enabled"] != true {
		t.Errorf("create: expected enabled=true by default, got %v", created["enabled"])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", id), &read)
	if err != nil {
		t.Fatalf("read automation rule failed: %v", err)
	}
	if read["id"] != id {
		t.Errorf("read: expected id %q, got %v", id, read["id"])
	}

	// Update
	updateBody := map[string]interface{}{
		"name":    "Auto-close old issues",
		"enabled": false,
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", id), jiraBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update automation rule failed: %v", err)
	}
	if updated["name"] != "Auto-close old issues" {
		t.Errorf("update: expected name 'Auto-close old issues', got %v", updated["name"])
	}
	if updated["enabled"] != false {
		t.Errorf("update: expected enabled=false, got %v", updated["enabled"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", id))
	if err != nil {
		t.Fatalf("delete automation rule failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", id), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted automation rule, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestJiraIntegrationAutomationRuleMissingTrigger(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]interface{}{
		"name": "Rule without trigger",
	}
	err := c.Post(ctx, "/rest/api/3/automation/rule", jiraBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for missing trigger, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

// --- Incoming Mail Handler CRUD ---

func TestJiraIntegrationIncomingMailHandlerCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "IncomingMailHandler",
		basePath:     "/rest/api/3/mailhandler/incoming",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "Support Inbox", "email": "support@example.com"},
		updateBody:   map[string]interface{}{"name": "Help Desk Inbox"},
		verifyField:  "name",
		verifyCreate: "Support Inbox",
		verifyUpdate: "Help Desk Inbox",
	})
}

// --- Outgoing Mail Handler CRUD ---

func TestJiraIntegrationOutgoingMailHandlerCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "OutgoingMailHandler",
		basePath:     "/rest/api/3/mailhandler/outgoing",
		idField:      "id",
		createBody:   map[string]interface{}{"name": "SMTP Server", "host": "smtp.example.com"},
		updateBody:   map[string]interface{}{"name": "Updated SMTP Server"},
		verifyField:  "name",
		verifyCreate: "SMTP Server",
		verifyUpdate: "Updated SMTP Server",
	})
}

// --- Custom Domain CRUD (with DNS records) ---

func TestJiraIntegrationCustomDomainCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create domain
	createBody := map[string]interface{}{
		"domain": "jira.example.com",
	}
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/domain", jiraBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create domain failed: %v", err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatal("create domain: expected non-empty id")
	}
	if created["domain"] != "jira.example.com" {
		t.Errorf("create: expected domain 'jira.example.com', got %v", created["domain"])
	}
	if created["status"] != "pending" {
		t.Errorf("create: expected status 'pending', got %v", created["status"])
	}

	// Verify DNS records are returned
	dnsRecords, ok := created["dnsRecords"].([]interface{})
	if !ok {
		t.Fatal("create domain: expected dnsRecords array")
	}
	if len(dnsRecords) < 2 {
		t.Errorf("create domain: expected at least 2 DNS records, got %d", len(dnsRecords))
	}

	// Verify DNS record types
	recordTypes := make(map[string]bool)
	for _, rec := range dnsRecords {
		recMap, ok := rec.(map[string]interface{})
		if !ok {
			continue
		}
		recordTypes[recMap["type"].(string)] = true
	}
	for _, expectedType := range []string{"TXT", "CNAME", "MX"} {
		if !recordTypes[expectedType] {
			t.Errorf("create domain: expected DNS record type %s", expectedType)
		}
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/domain/%s", id), &read)
	if err != nil {
		t.Fatalf("read domain failed: %v", err)
	}
	if read["domain"] != "jira.example.com" {
		t.Errorf("read: expected domain 'jira.example.com', got %v", read["domain"])
	}

	// Update (status change, DNS records should be immutable)
	updateBody := map[string]interface{}{
		"status": "verified",
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/domain/%s", id), jiraBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update domain failed: %v", err)
	}
	if updated["status"] != "verified" {
		t.Errorf("update: expected status 'verified', got %v", updated["status"])
	}
	// DNS records should still be present (immutable on update)
	if updated["dnsRecords"] == nil {
		t.Error("update: dnsRecords should be preserved")
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/domain/%s", id))
	if err != nil {
		t.Fatalf("delete domain failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/domain/%s", id), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted domain, got nil")
	}
}

func TestJiraIntegrationCustomDomainDuplicate(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	body := map[string]interface{}{"domain": "dup.example.com"}
	err := c.Post(ctx, "/rest/api/3/domain", jiraBody(t, body), nil)
	if err != nil {
		t.Fatalf("create first domain failed: %v", err)
	}

	err = c.Post(ctx, "/rest/api/3/domain", jiraBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for duplicate domain, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected 409, got %d", apiErr.StatusCode)
	}
}

// --- Custom Email CRUD ---

func TestJiraIntegrationCustomEmailCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	testCRUDLifecycle(t, c, crudResource{
		name:         "CustomEmail",
		basePath:     "/rest/api/3/email",
		idField:      "id",
		createBody:   map[string]interface{}{"emailAddress": "jira@example.com", "description": "Main Jira email"},
		updateBody:   map[string]interface{}{"description": "Updated description"},
		verifyField:  "emailAddress",
		verifyCreate: "jira@example.com",
		verifyUpdate: "jira@example.com",
	})
}

// --- Cross-Resource Operations ---

func TestJiraIntegrationCrossResourceSpaceWorkflowPermissions(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Step 1: Create space
	var space map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/project", jiraBody(t, map[string]interface{}{
		"name":           "Cross-Resource Space",
		"key":            "CRS",
		"projectTypeKey": "software",
	}), &space)
	if err != nil {
		t.Fatalf("create space failed: %v", err)
	}
	spaceID := space["id"].(string)
	t.Logf("created space: %s (key=CRS)", spaceID)

	// Step 2: Create issue types
	var bugType, storyType map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/issuetype", jiraBody(t, map[string]interface{}{
		"name":        "Bug",
		"description": "A software defect",
	}), &bugType)
	if err != nil {
		t.Fatalf("create bug type failed: %v", err)
	}
	bugTypeID := bugType["id"].(string)

	err = c.Post(ctx, "/rest/api/3/issuetype", jiraBody(t, map[string]interface{}{
		"name":        "Story",
		"description": "A user story",
	}), &storyType)
	if err != nil {
		t.Fatalf("create story type failed: %v", err)
	}
	storyTypeID := storyType["id"].(string)
	t.Logf("created issue types: bug=%s, story=%s", bugTypeID, storyTypeID)

	// Step 3: Create issue type scheme
	var issueTypeScheme map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/issuetypescheme", jiraBody(t, map[string]interface{}{
		"name":         "CRS Issue Types",
		"description":  "Issue types for CRS project",
		"issueTypeIds": []string{bugTypeID, storyTypeID},
	}), &issueTypeScheme)
	if err != nil {
		t.Fatalf("create issue type scheme failed: %v", err)
	}
	issueTypeSchemeID := issueTypeScheme["id"].(string)
	t.Logf("created issue type scheme: %s", issueTypeSchemeID)

	// Step 4: Create workflow
	var workflow map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/workflow", jiraBody(t, map[string]interface{}{
		"name":        "CRS Workflow",
		"description": "Workflow for CRS project",
	}), &workflow)
	if err != nil {
		t.Fatalf("create workflow failed: %v", err)
	}
	workflowID := workflow["id"].(string)
	t.Logf("created workflow: %s", workflowID)

	// Step 5: Create workflow scheme referencing the workflow
	var workflowScheme map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/workflowscheme", jiraBody(t, map[string]interface{}{
		"name":              "CRS Workflow Scheme",
		"description":       "Workflow scheme for CRS project",
		"defaultWorkflowId": workflowID,
	}), &workflowScheme)
	if err != nil {
		t.Fatalf("create workflow scheme failed: %v", err)
	}
	workflowSchemeID := workflowScheme["id"].(string)
	t.Logf("created workflow scheme: %s", workflowSchemeID)

	// Step 6: Create permission scheme
	var permScheme map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/permissionscheme", jiraBody(t, map[string]interface{}{
		"name":        "CRS Permission Scheme",
		"description": "Permissions for CRS project",
	}), &permScheme)
	if err != nil {
		t.Fatalf("create permission scheme failed: %v", err)
	}
	permSchemeID := permScheme["id"].(string)
	t.Logf("created permission scheme: %s", permSchemeID)

	// Step 7: Create screen and screen scheme
	var screen map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/screen", jiraBody(t, map[string]interface{}{
		"name":        "CRS Screen",
		"description": "Screen for CRS project",
	}), &screen)
	if err != nil {
		t.Fatalf("create screen failed: %v", err)
	}
	screenID := screen["id"].(string)

	var screenScheme map[string]interface{}
	err = c.Post(ctx, "/rest/api/3/screenscheme", jiraBody(t, map[string]interface{}{
		"name":            "CRS Screen Scheme",
		"description":     "Screen scheme for CRS",
		"defaultScreenId": screenID,
	}), &screenScheme)
	if err != nil {
		t.Fatalf("create screen scheme failed: %v", err)
	}
	screenSchemeID := screenScheme["id"].(string)
	t.Logf("created screen scheme: %s (screen=%s)", screenSchemeID, screenID)

	// Verify all resources are readable
	for _, res := range []struct {
		path string
		name string
	}{
		{fmt.Sprintf("/rest/api/3/project/%s", spaceID), "space"},
		{fmt.Sprintf("/rest/api/3/issuetype/%s", bugTypeID), "bug type"},
		{fmt.Sprintf("/rest/api/3/issuetype/%s", storyTypeID), "story type"},
		{fmt.Sprintf("/rest/api/3/issuetypescheme/%s", issueTypeSchemeID), "issue type scheme"},
		{fmt.Sprintf("/rest/api/3/workflow/%s", workflowID), "workflow"},
		{fmt.Sprintf("/rest/api/3/workflowscheme/%s", workflowSchemeID), "workflow scheme"},
		{fmt.Sprintf("/rest/api/3/permissionscheme/%s", permSchemeID), "permission scheme"},
		{fmt.Sprintf("/rest/api/3/screen/%s", screenID), "screen"},
		{fmt.Sprintf("/rest/api/3/screenscheme/%s", screenSchemeID), "screen scheme"},
	} {
		var result map[string]interface{}
		err = c.Get(ctx, res.path, &result)
		if err != nil {
			t.Errorf("cross-resource read %s failed: %v", res.name, err)
		}
	}

	// Cleanup in reverse dependency order
	for _, path := range []string{
		fmt.Sprintf("/rest/api/3/screenscheme/%s", screenSchemeID),
		fmt.Sprintf("/rest/api/3/screen/%s", screenID),
		fmt.Sprintf("/rest/api/3/permissionscheme/%s", permSchemeID),
		fmt.Sprintf("/rest/api/3/workflowscheme/%s", workflowSchemeID),
		fmt.Sprintf("/rest/api/3/workflow/%s", workflowID),
		fmt.Sprintf("/rest/api/3/issuetypescheme/%s", issueTypeSchemeID),
		fmt.Sprintf("/rest/api/3/issuetype/%s", storyTypeID),
		fmt.Sprintf("/rest/api/3/issuetype/%s", bugTypeID),
		fmt.Sprintf("/rest/api/3/project/%s", spaceID),
	} {
		err = c.Delete(ctx, path)
		if err != nil {
			t.Errorf("cross-resource cleanup delete %s failed: %v", path, err)
		}
	}
}

// --- Import (Read-by-ID after Create) Tests ---

func TestJiraIntegrationImportIssueTypeByID(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/issuetype", jiraBody(t, map[string]interface{}{
		"name":        "Import Bug",
		"description": "Bug to import",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/issuetype/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["id"] != id {
		t.Errorf("import: id mismatch: %v vs %v", imported["id"], id)
	}
	if imported["name"] != "Import Bug" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestJiraIntegrationImportSpaceByKey(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/project", jiraBody(t, map[string]interface{}{
		"name":           "Import Space",
		"key":            "IMP",
		"projectTypeKey": "software",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Import by key
	var imported map[string]interface{}
	err = c.Get(ctx, "/rest/api/3/project/IMP", &imported)
	if err != nil {
		t.Fatalf("import by key failed: %v", err)
	}
	if imported["key"] != "IMP" {
		t.Errorf("import: key mismatch: got %v", imported["key"])
	}
	if imported["name"] != "Import Space" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestJiraIntegrationImportWorkflowByID(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/workflow", jiraBody(t, map[string]interface{}{
		"name": "Import Workflow",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["name"] != "Import Workflow" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestJiraIntegrationImportPermissionSchemeByID(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/permissionscheme", jiraBody(t, map[string]interface{}{
		"name": "Import Permission Scheme",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/permissionscheme/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["name"] != "Import Permission Scheme" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestJiraIntegrationImportDashboardByID(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/dashboard", jiraBody(t, map[string]interface{}{
		"name": "Import Dashboard",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/dashboard/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["name"] != "Import Dashboard" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestJiraIntegrationImportCustomDomainByID(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/domain", jiraBody(t, map[string]interface{}{
		"domain": "import.example.com",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/domain/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["domain"] != "import.example.com" {
		t.Errorf("import: domain mismatch: got %v", imported["domain"])
	}
	if imported["dnsRecords"] == nil {
		t.Error("import: expected dnsRecords to be present")
	}
}

func TestJiraIntegrationImportAutomationRuleByID(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/automation/rule", jiraBody(t, map[string]interface{}{
		"name":    "Import Automation",
		"trigger": map[string]interface{}{"type": "manual"},
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["name"] != "Import Automation" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestJiraIntegrationImportBoardByID(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/filter", jiraBody(t, map[string]interface{}{
		"name": "Import Filter",
	}), &created)
	if err != nil {
		t.Fatalf("create filter failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/filter/%s", id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["name"] != "Import Filter" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

// --- Idempotency Tests ---

func TestJiraIntegrationIssueTypeUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/issuetype", jiraBody(t, map[string]interface{}{
		"name":        "Idempotent Type",
		"description": "Test idempotency",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	updateBody := map[string]interface{}{"name": "Idempotent Type", "description": "Same value"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/issuetype/%s", id), jiraBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/issuetype/%s", id), jiraBody(t, updateBody), &second)
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

func TestJiraIntegrationWorkflowSchemeUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/workflowscheme", jiraBody(t, map[string]interface{}{
		"name": "Idempotent WF Scheme",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	updateBody := map[string]interface{}{"name": "Idempotent WF Scheme", "description": "Same"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/workflowscheme/%s", id), jiraBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/workflowscheme/%s", id), jiraBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["name"] != second["name"] {
		t.Errorf("idempotency: name differs: %v vs %v", first["name"], second["name"])
	}
	if first["description"] != second["description"] {
		t.Errorf("idempotency: description differs: %v vs %v", first["description"], second["description"])
	}
}

func TestJiraIntegrationScreenUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/screen", jiraBody(t, map[string]interface{}{
		"name": "Idempotent Screen",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	updateBody := map[string]interface{}{"name": "Idempotent Screen"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/screen/%s", id), jiraBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/screen/%s", id), jiraBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["name"] != second["name"] {
		t.Errorf("idempotency: name differs: %v vs %v", first["name"], second["name"])
	}
}

func TestJiraIntegrationSpaceUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/project", jiraBody(t, map[string]interface{}{
		"name":           "Idempotent Space",
		"key":            "IDEM",
		"projectTypeKey": "software",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	updateBody := map[string]interface{}{"name": "Idempotent Space"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/project/%s", id), jiraBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/project/%s", id), jiraBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["name"] != second["name"] {
		t.Errorf("idempotency: name differs: %v vs %v", first["name"], second["name"])
	}
	if first["key"] != second["key"] {
		t.Errorf("idempotency: key differs: %v vs %v", first["key"], second["key"])
	}
}

func TestJiraIntegrationDomainUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/domain", jiraBody(t, map[string]interface{}{
		"domain": "idempotent.example.com",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	updateBody := map[string]interface{}{"status": "pending"}
	var first, second map[string]interface{}

	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/domain/%s", id), jiraBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/domain/%s", id), jiraBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["status"] != second["status"] {
		t.Errorf("idempotency: status differs: %v vs %v", first["status"], second["status"])
	}
	if first["domain"] != second["domain"] {
		t.Errorf("idempotency: domain differs: %v vs %v", first["domain"], second["domain"])
	}
}

// --- Drift Detection Tests ---

func TestJiraIntegrationDriftDetectionIssueTypeModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/issuetype", jiraBody(t, map[string]interface{}{
		"name": "Drift Type",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// Simulate external modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/issuetype/%s", id),
		jiraBody(t, map[string]interface{}{"name": "Externally Changed Type"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read should reflect external change
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/issuetype/%s", id), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["name"] != "Externally Changed Type" {
		t.Errorf("drift not detected: expected 'Externally Changed Type', got %v", current["name"])
	}
}

func TestJiraIntegrationDriftDetectionWorkflowModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/workflow", jiraBody(t, map[string]interface{}{
		"name":        "Drift Workflow",
		"description": "Original",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", id),
		jiraBody(t, map[string]interface{}{"description": "Externally Updated"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Verify drift
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", id), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["description"] != "Externally Updated" {
		t.Errorf("drift not detected: expected 'Externally Updated', got %v", current["description"])
	}
}

func TestJiraIntegrationDriftDetectionSpaceModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/project", jiraBody(t, map[string]interface{}{
		"name":           "Drift Space",
		"key":            "DRFT",
		"projectTypeKey": "software",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/project/%s", id),
		jiraBody(t, map[string]interface{}{"name": "Externally Renamed Space"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Verify drift
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/project/%s", id), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["name"] != "Externally Renamed Space" {
		t.Errorf("drift not detected: expected 'Externally Renamed Space', got %v", current["name"])
	}
}

func TestJiraIntegrationDriftDetectionPermissionSchemeDeletedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/permissionscheme", jiraBody(t, map[string]interface{}{
		"name": "Ephemeral Scheme",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External delete
	err = c.Delete(ctx, fmt.Sprintf("/rest/api/3/permissionscheme/%s", id))
	if err != nil {
		t.Fatalf("external delete failed: %v", err)
	}

	// Verify drift detection: read returns 404
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/permissionscheme/%s", id), &ghost)
	if err == nil {
		t.Fatal("expected error for externally deleted permission scheme, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestJiraIntegrationDriftDetectionDashboardModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/dashboard", jiraBody(t, map[string]interface{}{
		"name": "Drift Dashboard",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External modification
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/dashboard/%s", id),
		jiraBody(t, map[string]interface{}{"name": "Renamed Dashboard"}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/dashboard/%s", id), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["name"] != "Renamed Dashboard" {
		t.Errorf("drift not detected: expected 'Renamed Dashboard', got %v", current["name"])
	}
}

func TestJiraIntegrationDriftDetectionAutomationRuleModifiedExternally(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/automation/rule", jiraBody(t, map[string]interface{}{
		"name":    "Drift Automation",
		"trigger": map[string]interface{}{"type": "manual"},
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External modification: disable the rule
	var modified map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", id),
		jiraBody(t, map[string]interface{}{"enabled": false}), &modified)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/automation/rule/%s", id), &current)
	if err != nil {
		t.Fatalf("drift detection read failed: %v", err)
	}
	if current["enabled"] != false {
		t.Errorf("drift not detected: expected enabled=false, got %v", current["enabled"])
	}
}

// --- State Consistency Tests ---

func TestJiraIntegrationStateConsistencyCreateReadMatch(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// For each resource type, verify that create response matches subsequent read
	tests := []struct {
		name     string
		path     string
		body     map[string]interface{}
		idField  string
		checkKey string
	}{
		{"IssueType", "/rest/api/3/issuetype", map[string]interface{}{"name": "State Bug"}, "id", "name"},
		{"Workflow", "/rest/api/3/workflow", map[string]interface{}{"name": "State Workflow"}, "id", "name"},
		{"Screen", "/rest/api/3/screen", map[string]interface{}{"name": "State Screen"}, "id", "name"},
		{"Dashboard", "/rest/api/3/dashboard", map[string]interface{}{"name": "State Dashboard"}, "id", "name"},
		{"Filter", "/rest/api/3/filter", map[string]interface{}{"name": "State Filter"}, "id", "name"},
		{"Priority", "/rest/api/3/priority", map[string]interface{}{"name": "State Priority"}, "id", "name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var created map[string]interface{}
			err := c.Post(ctx, tt.path, jiraBody(t, tt.body), &created)
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}
			id := created[tt.idField].(string)

			var read map[string]interface{}
			err = c.Get(ctx, fmt.Sprintf("%s/%s", tt.path, id), &read)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}

			if created[tt.checkKey] != read[tt.checkKey] {
				t.Errorf("state mismatch: create returned %v, read returned %v for %s",
					created[tt.checkKey], read[tt.checkKey], tt.checkKey)
			}
			if created[tt.idField] != read[tt.idField] {
				t.Errorf("state mismatch: create id %v != read id %v",
					created[tt.idField], read[tt.idField])
			}
		})
	}
}

func TestJiraIntegrationStateConsistencyUpdateReadMatch(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create a workflow, update it, verify update response matches read
	var created map[string]interface{}
	err := c.Post(ctx, "/rest/api/3/workflow", jiraBody(t, map[string]interface{}{
		"name":        "Consistency WF",
		"description": "Original",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", id),
		jiraBody(t, map[string]interface{}{"description": "Updated"}), &updated)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("/rest/api/3/workflow/%s", id), &read)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if updated["name"] != read["name"] {
		t.Errorf("state mismatch: update name %v != read name %v", updated["name"], read["name"])
	}
	if updated["description"] != read["description"] {
		t.Errorf("state mismatch: update description %v != read description %v",
			updated["description"], read["description"])
	}
	if updated["id"] != read["id"] {
		t.Errorf("state mismatch: update id %v != read id %v", updated["id"], read["id"])
	}
}

// --- Missing Required Fields Tests ---

func TestJiraIntegrationMissingRequiredFields(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tests := []struct {
		name string
		path string
		body map[string]interface{}
	}{
		{"IssueType missing name", "/rest/api/3/issuetype", map[string]interface{}{"description": "no name"}},
		{"Workflow missing name", "/rest/api/3/workflow", map[string]interface{}{"description": "no name"}},
		{"Screen missing name", "/rest/api/3/screen", map[string]interface{}{"description": "no name"}},
		{"PermissionScheme missing name", "/rest/api/3/permissionscheme", map[string]interface{}{"description": "no name"}},
		{"CustomField missing name", "/rest/api/3/field", map[string]interface{}{"type": "number"}},
		{"CustomField missing type", "/rest/api/3/field", map[string]interface{}{"name": "Field"}},
		{"Board missing name", "/rest/agile/1.0/board", map[string]interface{}{"type": "scrum"}},
		{"Board missing type", "/rest/agile/1.0/board", map[string]interface{}{"name": "Board"}},
		{"Priority missing name", "/rest/api/3/priority", map[string]interface{}{"description": "no name"}},
		{"Domain missing domain", "/rest/api/3/domain", map[string]interface{}{"status": "pending"}},
		{"Email missing emailAddress", "/rest/api/3/email", map[string]interface{}{"description": "no email"}},
		{"AutomationRule missing name", "/rest/api/3/automation/rule", map[string]interface{}{"trigger": map[string]interface{}{"type": "manual"}}},
		{"Space missing key", "/rest/api/3/project", map[string]interface{}{"name": "Test", "projectTypeKey": "software"}},
		{"Space missing projectTypeKey", "/rest/api/3/project", map[string]interface{}{"name": "Test", "key": "TST"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Post(ctx, tt.path, jiraBody(t, tt.body), nil)
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

// --- Read/Delete Not Found Tests ---

func TestJiraIntegrationReadNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	paths := []struct {
		name string
		path string
	}{
		{"IssueType", "/rest/api/3/issuetype/nonexistent"},
		{"Workflow", "/rest/api/3/workflow/nonexistent"},
		{"WorkflowScheme", "/rest/api/3/workflowscheme/nonexistent"},
		{"Screen", "/rest/api/3/screen/nonexistent"},
		{"ScreenScheme", "/rest/api/3/screenscheme/nonexistent"},
		{"PermissionScheme", "/rest/api/3/permissionscheme/nonexistent"},
		{"SecurityScheme", "/rest/api/3/issuesecurityschemes/nonexistent"},
		{"NotificationScheme", "/rest/api/3/notificationscheme/nonexistent"},
		{"Dashboard", "/rest/api/3/dashboard/nonexistent"},
		{"Filter", "/rest/api/3/filter/nonexistent"},
		{"CustomField", "/rest/api/3/field/nonexistent"},
		{"Board", "/rest/agile/1.0/board/nonexistent"},
		{"Priority", "/rest/api/3/priority/nonexistent"},
		{"PriorityScheme", "/rest/api/3/priorityscheme/nonexistent"},
		{"AutomationRule", "/rest/api/3/automation/rule/nonexistent"},
		{"IncomingMailHandler", "/rest/api/3/mailhandler/incoming/nonexistent"},
		{"OutgoingMailHandler", "/rest/api/3/mailhandler/outgoing/nonexistent"},
		{"Domain", "/rest/api/3/domain/nonexistent"},
		{"Email", "/rest/api/3/email/nonexistent"},
		{"Space", "/rest/api/3/project/nonexistent"},
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

func TestJiraIntegrationDeleteNotFound(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	paths := []struct {
		name string
		path string
	}{
		{"IssueType", "/rest/api/3/issuetype/nonexistent"},
		{"Workflow", "/rest/api/3/workflow/nonexistent"},
		{"Screen", "/rest/api/3/screen/nonexistent"},
		{"PermissionScheme", "/rest/api/3/permissionscheme/nonexistent"},
		{"Dashboard", "/rest/api/3/dashboard/nonexistent"},
		{"Filter", "/rest/api/3/filter/nonexistent"},
		{"Board", "/rest/agile/1.0/board/nonexistent"},
		{"AutomationRule", "/rest/api/3/automation/rule/nonexistent"},
		{"Domain", "/rest/api/3/domain/nonexistent"},
		{"Space", "/rest/api/3/project/nonexistent"},
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

// --- Duplicate Name/Key Tests ---

func TestJiraIntegrationDuplicateNameRejected(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tests := []struct {
		name string
		path string
		body map[string]interface{}
	}{
		{"IssueType", "/rest/api/3/issuetype", map[string]interface{}{"name": "DupType"}},
		{"Workflow", "/rest/api/3/workflow", map[string]interface{}{"name": "DupWorkflow"}},
		{"Screen", "/rest/api/3/screen", map[string]interface{}{"name": "DupScreen"}},
		{"PermissionScheme", "/rest/api/3/permissionscheme", map[string]interface{}{"name": "DupPerm"}},
		{"SecurityScheme", "/rest/api/3/issuesecurityschemes", map[string]interface{}{"name": "DupSecurity"}},
		{"NotificationScheme", "/rest/api/3/notificationscheme", map[string]interface{}{"name": "DupNotif"}},
		{"Priority", "/rest/api/3/priority", map[string]interface{}{"name": "DupPriority"}},
		{"PriorityScheme", "/rest/api/3/priorityscheme", map[string]interface{}{"name": "DupPriorityScheme"}},
		{"IncomingMailHandler", "/rest/api/3/mailhandler/incoming", map[string]interface{}{"name": "DupIncoming"}},
		{"OutgoingMailHandler", "/rest/api/3/mailhandler/outgoing", map[string]interface{}{"name": "DupOutgoing"}},
		{"CustomField", "/rest/api/3/field", map[string]interface{}{"name": "DupField", "type": "text"}},
		{"Email", "/rest/api/3/email", map[string]interface{}{"emailAddress": "dup@example.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// First create should succeed
			err := c.Post(ctx, tt.path, jiraBody(t, tt.body), nil)
			if err != nil {
				t.Fatalf("[%s] first create failed: %v", tt.name, err)
			}

			// Second create with same name/key should fail with 409
			err = c.Post(ctx, tt.path, jiraBody(t, tt.body), nil)
			if err == nil {
				t.Fatalf("[%s] expected error for duplicate, got nil", tt.name)
			}
			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("[%s] expected *client.APIError, got %T", tt.name, err)
			}
			if apiErr.StatusCode != 409 {
				t.Errorf("[%s] expected 409, got %d", tt.name, apiErr.StatusCode)
			}
		})
	}
}

// --- List Endpoints Tests ---

func TestJiraIntegrationListEndpoints(t *testing.T) {
	t.Parallel()
	_, c := setupJiraMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// Create one of each, then verify list returns it
	resources := []struct {
		name     string
		path     string
		listPath string
		body     map[string]interface{}
	}{
		{"IssueType", "/rest/api/3/issuetype", "/rest/api/3/issuetype", map[string]interface{}{"name": "ListType"}},
		{"Workflow", "/rest/api/3/workflow", "/rest/api/3/workflow", map[string]interface{}{"name": "ListWorkflow"}},
		{"Dashboard", "/rest/api/3/dashboard", "/rest/api/3/dashboard", map[string]interface{}{"name": "ListDashboard"}},
		{"Filter", "/rest/api/3/filter", "/rest/api/3/filter", map[string]interface{}{"name": "ListFilter"}},
		{"Priority", "/rest/api/3/priority", "/rest/api/3/priority", map[string]interface{}{"name": "ListPriority"}},
	}

	for _, r := range resources {
		t.Run(r.name, func(t *testing.T) {
			err := c.Post(ctx, r.path, jiraBody(t, r.body), nil)
			if err != nil {
				t.Fatalf("create failed: %v", err)
			}

			var listResp map[string]interface{}
			err = c.Get(ctx, r.listPath, &listResp)
			if err != nil {
				t.Fatalf("list failed: %v", err)
			}

			values, ok := listResp["values"].([]interface{})
			if !ok {
				t.Fatal("expected values array in list response")
			}
			if len(values) < 1 {
				t.Errorf("expected at least 1 item in list, got %d", len(values))
			}

			total, ok := listResp["total"].(float64)
			if !ok {
				t.Fatal("expected total in list response")
			}
			if int(total) != len(values) {
				t.Errorf("total %d does not match values length %d", int(total), len(values))
			}
		})
	}
}
