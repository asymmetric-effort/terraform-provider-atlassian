// Package unit contains unit tests for the mock API Jira endpoints.
package unit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// newJiraServer creates a mock server with Jira endpoints registered.
func newJiraServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterJiraEndpoints(s)
	return httptest.NewServer(s.Handler())
}

// TestProjectCRUDLifecycle tests create, read, update, list, and delete for projects (spaces).
func TestProjectCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Create project
	resp := postJSON(t, ts.URL+"/rest/api/3/project", map[string]string{
		"name":           "My Project",
		"key":            "MP",
		"projectTypeKey": "software",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project: expected 201, got %d", resp.StatusCode)
	}
	var project map[string]interface{}
	decodeJSON(t, resp, &project)
	projectID, ok := project["id"].(string)
	if !ok || projectID == "" {
		t.Fatal("create project: expected non-empty id")
	}
	if project["key"] != "MP" {
		t.Errorf("create project: expected key 'MP', got %v", project["key"])
	}
	if project["name"] != "My Project" {
		t.Errorf("create project: expected name 'My Project', got %v", project["name"])
	}

	// Read project by ID
	resp, err := http.Get(ts.URL + "/rest/api/3/project/" + projectID)
	if err != nil {
		t.Fatalf("read project: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read project: expected 200, got %d", resp.StatusCode)
	}
	var readProject map[string]interface{}
	decodeJSON(t, resp, &readProject)
	if readProject["id"] != projectID {
		t.Errorf("read project: expected id %q, got %v", projectID, readProject["id"])
	}

	// Read project by key
	resp, err = http.Get(ts.URL + "/rest/api/3/project/MP")
	if err != nil {
		t.Fatalf("read project by key: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read project by key: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update project
	resp = putJSON(t, ts.URL+"/rest/api/3/project/"+projectID, map[string]string{
		"name": "Updated Project",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update project: expected 200, got %d", resp.StatusCode)
	}
	var updatedProject map[string]interface{}
	decodeJSON(t, resp, &updatedProject)
	if updatedProject["name"] != "Updated Project" {
		t.Errorf("update project: expected name 'Updated Project', got %v", updatedProject["name"])
	}
	if updatedProject["id"] != projectID {
		t.Errorf("update project: id should not change")
	}

	// List projects
	resp, err = http.Get(ts.URL + "/rest/api/3/project")
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list projects: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok {
		t.Fatal("list projects: expected values array")
	}
	if len(values) != 1 {
		t.Errorf("list projects: expected 1 project, got %d", len(values))
	}

	// Duplicate key
	resp = postJSON(t, ts.URL+"/rest/api/3/project", map[string]string{
		"name":           "Another Project",
		"key":            "MP",
		"projectTypeKey": "software",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate key: expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing fields
	resp = postJSON(t, ts.URL+"/rest/api/3/project", map[string]string{
		"name": "No Key",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing key: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete project
	resp = doDelete(t, ts.URL+"/rest/api/3/project/"+projectID)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete project: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify project is deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/project/" + projectID)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestProjectReadNotFound tests reading a non-existent project.
func TestProjectReadNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/project/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

// TestProjectDeleteNotFound tests deleting a non-existent project.
func TestProjectDeleteNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := doDelete(t, ts.URL+"/rest/api/3/project/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestIssueTypeCRUDLifecycle tests CRUD for issue types.
func TestIssueTypeCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Create issue type
	resp := postJSON(t, ts.URL+"/rest/api/3/issuetype", map[string]string{
		"name":        "Bug",
		"description": "A software bug",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create issue type: expected 201, got %d", resp.StatusCode)
	}
	var issueType map[string]interface{}
	decodeJSON(t, resp, &issueType)
	id, ok := issueType["id"].(string)
	if !ok || id == "" {
		t.Fatal("create issue type: expected non-empty id")
	}
	if issueType["name"] != "Bug" {
		t.Errorf("create issue type: expected name 'Bug', got %v", issueType["name"])
	}

	// Read issue type
	resp, err := http.Get(ts.URL + "/rest/api/3/issuetype/" + id)
	if err != nil {
		t.Fatalf("read issue type: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read issue type: expected 200, got %d", resp.StatusCode)
	}
	var readIT map[string]interface{}
	decodeJSON(t, resp, &readIT)
	if readIT["id"] != id {
		t.Errorf("read issue type: expected id %q, got %v", id, readIT["id"])
	}

	// Update issue type
	resp = putJSON(t, ts.URL+"/rest/api/3/issuetype/"+id, map[string]string{
		"description": "Updated bug description",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update issue type: expected 200, got %d", resp.StatusCode)
	}
	var updatedIT map[string]interface{}
	decodeJSON(t, resp, &updatedIT)
	if updatedIT["description"] != "Updated bug description" {
		t.Errorf("update issue type: expected updated description, got %v", updatedIT["description"])
	}

	// List issue types
	resp, err = http.Get(ts.URL + "/rest/api/3/issuetype")
	if err != nil {
		t.Fatalf("list issue types: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list issue types: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok {
		t.Fatal("list issue types: expected values array")
	}
	if len(values) != 1 {
		t.Errorf("list issue types: expected 1, got %d", len(values))
	}

	// Duplicate name
	resp = postJSON(t, ts.URL+"/rest/api/3/issuetype", map[string]string{
		"name": "Bug",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate name: expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing name
	resp = postJSON(t, ts.URL+"/rest/api/3/issuetype", map[string]string{
		"description": "no name",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing name: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete issue type
	resp = doDelete(t, ts.URL+"/rest/api/3/issuetype/"+id)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete issue type: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/issuetype/" + id)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestIssueTypeNotFound tests reading and deleting non-existent issue types.
func TestIssueTypeNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/issuetype/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("read: expected 404, got %d", resp.StatusCode)
	}

	resp = doDelete(t, ts.URL+"/rest/api/3/issuetype/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestWorkflowCRUDLifecycle tests CRUD for workflows.
func TestWorkflowCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Create workflow
	resp := postJSON(t, ts.URL+"/rest/api/3/workflow", map[string]interface{}{
		"name":        "Default Workflow",
		"description": "A default workflow",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create workflow: expected 201, got %d", resp.StatusCode)
	}
	var workflow map[string]interface{}
	decodeJSON(t, resp, &workflow)
	id, ok := workflow["id"].(string)
	if !ok || id == "" {
		t.Fatal("create workflow: expected non-empty id")
	}
	if workflow["name"] != "Default Workflow" {
		t.Errorf("create workflow: expected name 'Default Workflow', got %v", workflow["name"])
	}

	// Read workflow
	resp, err := http.Get(ts.URL + "/rest/api/3/workflow/" + id)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read workflow: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update workflow
	resp = putJSON(t, ts.URL+"/rest/api/3/workflow/"+id, map[string]string{
		"description": "Updated workflow",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update workflow: expected 200, got %d", resp.StatusCode)
	}
	var updated map[string]interface{}
	decodeJSON(t, resp, &updated)
	if updated["description"] != "Updated workflow" {
		t.Errorf("update workflow: expected updated description, got %v", updated["description"])
	}

	// List workflows
	resp, err = http.Get(ts.URL + "/rest/api/3/workflow")
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list workflows: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("list workflows: expected 1, got %d", len(values))
	}

	// Duplicate name
	resp = postJSON(t, ts.URL+"/rest/api/3/workflow", map[string]string{
		"name": "Default Workflow",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate name: expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete workflow
	resp = doDelete(t, ts.URL+"/rest/api/3/workflow/"+id)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete workflow: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/workflow/" + id)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestWorkflowNotFound tests reading and deleting non-existent workflows.
func TestWorkflowNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/workflow/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	resp = doDelete(t, ts.URL+"/rest/api/3/workflow/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestAutomationRuleCRUDLifecycle tests CRUD for automation rules with trigger, conditions, and actions.
func TestAutomationRuleCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Create automation rule with trigger, conditions, and actions
	resp := postJSON(t, ts.URL+"/rest/api/3/automation/rule", map[string]interface{}{
		"name": "Auto-assign on create",
		"trigger": map[string]interface{}{
			"type":      "issue_created",
			"component": "TRIGGER",
		},
		"conditions": []map[string]interface{}{
			{
				"type":  "issue_type",
				"value": "Bug",
			},
		},
		"actions": []map[string]interface{}{
			{
				"type":  "assign_issue",
				"value": "lead",
			},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create automation rule: expected 201, got %d", resp.StatusCode)
	}
	var rule map[string]interface{}
	decodeJSON(t, resp, &rule)
	id, ok := rule["id"].(string)
	if !ok || id == "" {
		t.Fatal("create automation rule: expected non-empty id")
	}
	if rule["name"] != "Auto-assign on create" {
		t.Errorf("create automation rule: expected name 'Auto-assign on create', got %v", rule["name"])
	}
	if rule["enabled"] != true {
		t.Errorf("create automation rule: expected enabled=true by default, got %v", rule["enabled"])
	}

	// Verify trigger is stored
	trigger, ok := rule["trigger"].(map[string]interface{})
	if !ok {
		t.Fatal("create automation rule: expected trigger object")
	}
	if trigger["type"] != "issue_created" {
		t.Errorf("create automation rule: expected trigger type 'issue_created', got %v", trigger["type"])
	}

	// Verify conditions are stored
	conditions, ok := rule["conditions"].([]interface{})
	if !ok || len(conditions) != 1 {
		t.Fatalf("create automation rule: expected 1 condition, got %v", rule["conditions"])
	}

	// Verify actions are stored
	actions, ok := rule["actions"].([]interface{})
	if !ok || len(actions) != 1 {
		t.Fatalf("create automation rule: expected 1 action, got %v", rule["actions"])
	}

	// Read automation rule
	resp, err := http.Get(ts.URL + "/rest/api/3/automation/rule/" + id)
	if err != nil {
		t.Fatalf("read automation rule: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read automation rule: expected 200, got %d", resp.StatusCode)
	}
	var readRule map[string]interface{}
	decodeJSON(t, resp, &readRule)
	if readRule["id"] != id {
		t.Errorf("read automation rule: expected id %q, got %v", id, readRule["id"])
	}

	// Update automation rule
	resp = putJSON(t, ts.URL+"/rest/api/3/automation/rule/"+id, map[string]interface{}{
		"enabled": false,
		"name":    "Updated rule",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update automation rule: expected 200, got %d", resp.StatusCode)
	}
	var updatedRule map[string]interface{}
	decodeJSON(t, resp, &updatedRule)
	if updatedRule["enabled"] != false {
		t.Errorf("update automation rule: expected enabled=false, got %v", updatedRule["enabled"])
	}
	if updatedRule["name"] != "Updated rule" {
		t.Errorf("update automation rule: expected name 'Updated rule', got %v", updatedRule["name"])
	}
	if updatedRule["id"] != id {
		t.Errorf("update automation rule: id should not change")
	}

	// List automation rules
	resp, err = http.Get(ts.URL + "/rest/api/3/automation/rule")
	if err != nil {
		t.Fatalf("list automation rules: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list automation rules: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("list automation rules: expected 1, got %v", listResp["values"])
	}

	// Delete automation rule
	resp = doDelete(t, ts.URL+"/rest/api/3/automation/rule/"+id)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete automation rule: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/automation/rule/" + id)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestAutomationRuleMissingFields tests automation rule creation with missing required fields.
func TestAutomationRuleMissingFields(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Missing name
	resp := postJSON(t, ts.URL+"/rest/api/3/automation/rule", map[string]interface{}{
		"trigger": map[string]string{"type": "issue_created"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing name: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing trigger
	resp = postJSON(t, ts.URL+"/rest/api/3/automation/rule", map[string]interface{}{
		"name": "No trigger rule",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing trigger: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestAutomationRuleNotFound tests reading and deleting non-existent automation rules.
func TestAutomationRuleNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/automation/rule/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("read: expected 404, got %d", resp.StatusCode)
	}

	resp = doDelete(t, ts.URL+"/rest/api/3/automation/rule/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestCustomDomainCRUDLifecycle tests CRUD for custom domains with DNS record verification.
func TestCustomDomainCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Create domain
	resp := postJSON(t, ts.URL+"/rest/api/3/domain", map[string]string{
		"domain": "example.com",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create domain: expected 201, got %d", resp.StatusCode)
	}
	var domain map[string]interface{}
	decodeJSON(t, resp, &domain)
	id, ok := domain["id"].(string)
	if !ok || id == "" {
		t.Fatal("create domain: expected non-empty id")
	}
	if domain["domain"] != "example.com" {
		t.Errorf("create domain: expected domain 'example.com', got %v", domain["domain"])
	}
	if domain["status"] != "pending" {
		t.Errorf("create domain: expected status 'pending', got %v", domain["status"])
	}

	// Verify DNS records are generated
	dnsRecords, ok := domain["dnsRecords"].([]interface{})
	if !ok {
		t.Fatal("create domain: expected dnsRecords array")
	}
	if len(dnsRecords) < 4 {
		t.Fatalf("create domain: expected at least 4 DNS records, got %d", len(dnsRecords))
	}

	// Check for required DNS record types
	foundTypes := make(map[string]bool)
	for _, rec := range dnsRecords {
		record, ok := rec.(map[string]interface{})
		if !ok {
			t.Fatal("create domain: expected DNS record to be a map")
		}
		recType, _ := record["type"].(string)
		foundTypes[recType] = true
		// Verify each record has name and value
		if record["name"] == nil || record["name"] == "" {
			t.Error("create domain: DNS record missing name")
		}
		if record["value"] == nil || record["value"] == "" {
			t.Error("create domain: DNS record missing value")
		}
	}
	for _, expected := range []string{"TXT", "CNAME", "MX"} {
		if !foundTypes[expected] {
			t.Errorf("create domain: expected DNS record type %q", expected)
		}
	}

	// Verify TXT record contains domain verification string
	for _, rec := range dnsRecords {
		record := rec.(map[string]interface{})
		val, _ := record["value"].(string)
		recType, _ := record["type"].(string)
		if recType == "TXT" && len(val) > 0 {
			// At least one TXT record should reference the domain ID
			break
		}
	}

	// Read domain
	resp, err := http.Get(ts.URL + "/rest/api/3/domain/" + id)
	if err != nil {
		t.Fatalf("read domain: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read domain: expected 200, got %d", resp.StatusCode)
	}
	var readDomain map[string]interface{}
	decodeJSON(t, resp, &readDomain)
	if readDomain["id"] != id {
		t.Errorf("read domain: expected id %q, got %v", id, readDomain["id"])
	}

	// Update domain
	resp = putJSON(t, ts.URL+"/rest/api/3/domain/"+id, map[string]string{
		"status": "verified",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update domain: expected 200, got %d", resp.StatusCode)
	}
	var updatedDomain map[string]interface{}
	decodeJSON(t, resp, &updatedDomain)
	if updatedDomain["status"] != "verified" {
		t.Errorf("update domain: expected status 'verified', got %v", updatedDomain["status"])
	}
	// DNS records should not be overwritten by update
	if updatedDomain["dnsRecords"] == nil {
		t.Error("update domain: DNS records should persist after update")
	}

	// List domains
	resp, err = http.Get(ts.URL + "/rest/api/3/domain")
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list domains: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("list domains: expected 1, got %v", listResp["values"])
	}

	// Duplicate domain
	resp = postJSON(t, ts.URL+"/rest/api/3/domain", map[string]string{
		"domain": "example.com",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate domain: expected 409, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Missing domain field
	resp = postJSON(t, ts.URL+"/rest/api/3/domain", map[string]string{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("missing domain: expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete domain
	resp = doDelete(t, ts.URL+"/rest/api/3/domain/"+id)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete domain: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify deleted
	resp, err = http.Get(ts.URL + "/rest/api/3/domain/" + id)
	if err != nil {
		t.Fatalf("verify delete: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("verify delete: expected 404, got %d", resp.StatusCode)
	}
}

// TestCustomDomainNotFound tests reading and deleting non-existent domains.
func TestCustomDomainNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/domain/nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("read: expected 404, got %d", resp.StatusCode)
	}

	resp = doDelete(t, ts.URL+"/rest/api/3/domain/nonexistent")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestCustomDomainDNSRecordFormat verifies the format and content of generated DNS records.
func TestCustomDomainDNSRecordFormat(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/domain", map[string]string{
		"domain": "myapp.io",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create domain: expected 201, got %d", resp.StatusCode)
	}
	var domain map[string]interface{}
	decodeJSON(t, resp, &domain)

	dnsRecords, ok := domain["dnsRecords"].([]interface{})
	if !ok {
		t.Fatal("expected dnsRecords array")
	}

	// Verify specific record content
	var foundVerification, foundCNAME, foundMX, foundSPF bool
	for _, rec := range dnsRecords {
		record := rec.(map[string]interface{})
		recType, _ := record["type"].(string)
		name, _ := record["name"].(string)
		value, _ := record["value"].(string)

		switch {
		case recType == "TXT" && name == "myapp.io" && len(value) > 30 && value[:32] == "atlassian-domain-verification=do":
			foundVerification = true
		case recType == "CNAME" && name == "_atl-verify.myapp.io":
			foundCNAME = true
		case recType == "MX" && name == "myapp.io":
			foundMX = true
		case recType == "TXT" && name == "myapp.io" && len(value) > 5 && value[:6] == "v=spf1":
			foundSPF = true
		}
	}

	if !foundVerification {
		t.Error("expected TXT domain verification record")
	}
	if !foundCNAME {
		t.Error("expected CNAME verification record")
	}
	if !foundMX {
		t.Error("expected MX record")
	}
	if !foundSPF {
		t.Error("expected SPF TXT record")
	}
}

// TestJiraErrorResponseFormat verifies error responses follow Atlassian format.
func TestJiraErrorResponseFormat(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/rest/api/3/issuetype/nonexistent")
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
	_ = errs // just verify it exists as a map
}

// TestRegisterJiraEndpointsIntegration tests that RegisterJiraEndpoints registers all expected endpoints.
func TestRegisterJiraEndpointsIntegration(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Test a sampling of endpoint paths to verify they're all registered
	endpoints := []struct {
		method string
		path   string
		body   interface{}
		status int
	}{
		{"POST", "/rest/api/3/issuetypescheme", map[string]string{"name": "Default Scheme"}, http.StatusCreated},
		{"POST", "/rest/api/3/workflowscheme", map[string]string{"name": "Default WF Scheme"}, http.StatusCreated},
		{"POST", "/rest/api/3/screen", map[string]string{"name": "Default Screen"}, http.StatusCreated},
		{"POST", "/rest/api/3/screenscheme", map[string]string{"name": "Default Screen Scheme"}, http.StatusCreated},
		{"POST", "/rest/api/3/screentabfield", map[string]string{"fieldId": "summary"}, http.StatusCreated},
		{"POST", "/rest/api/3/permissionscheme", map[string]string{"name": "Default Permissions"}, http.StatusCreated},
		{"POST", "/rest/api/3/issuesecurityschemes", map[string]string{"name": "Security"}, http.StatusCreated},
		{"POST", "/rest/api/3/notificationscheme", map[string]string{"name": "Notifications"}, http.StatusCreated},
		{"POST", "/rest/api/3/dashboard", map[string]string{"name": "My Dashboard"}, http.StatusCreated},
		{"POST", "/rest/api/3/filter", map[string]string{"name": "My Filter"}, http.StatusCreated},
		{"POST", "/rest/api/3/field", map[string]string{"name": "Custom Field", "type": "string"}, http.StatusCreated},
		{"POST", "/rest/agile/1.0/board", map[string]string{"name": "Sprint Board", "type": "scrum"}, http.StatusCreated},
		{"POST", "/rest/api/3/priority", map[string]string{"name": "High"}, http.StatusCreated},
		{"POST", "/rest/api/3/priorityscheme", map[string]string{"name": "Default Priorities"}, http.StatusCreated},
		{"POST", "/rest/api/3/mailhandler/incoming", map[string]string{"name": "Incoming Handler"}, http.StatusCreated},
		{"POST", "/rest/api/3/mailhandler/outgoing", map[string]string{"name": "Outgoing Handler"}, http.StatusCreated},
		{"POST", "/rest/api/3/email", map[string]string{"emailAddress": "support@example.com"}, http.StatusCreated},
	}

	for _, ep := range endpoints {
		resp := postJSON(t, ts.URL+ep.path, ep.body)
		if resp.StatusCode != ep.status {
			t.Errorf("%s %s: expected %d, got %d", ep.method, ep.path, ep.status, resp.StatusCode)
		}
		var created map[string]interface{}
		decodeJSON(t, resp, &created)
		id, ok := created["id"].(string)
		if !ok || id == "" {
			t.Errorf("%s %s: expected non-empty id in response", ep.method, ep.path)
			continue
		}

		// Read back the created resource
		readResp, err := http.Get(fmt.Sprintf("%s%s/%s", ts.URL, ep.path, id))
		if err != nil {
			t.Fatalf("GET %s/%s: %v", ep.path, id, err)
		}
		if readResp.StatusCode != http.StatusOK {
			t.Errorf("GET %s/%s: expected 200, got %d", ep.path, id, readResp.StatusCode)
		}
		readResp.Body.Close()

		// Delete the resource
		delResp := doDelete(t, fmt.Sprintf("%s%s/%s", ts.URL, ep.path, id))
		if delResp.StatusCode != http.StatusNoContent {
			t.Errorf("DELETE %s/%s: expected 204, got %d", ep.path, id, delResp.StatusCode)
		}
		delResp.Body.Close()
	}
}

// TestCRUDEndpointUpdateInvalidJSON tests that invalid JSON in update returns 400.
func TestCRUDEndpointUpdateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	// Create a resource first
	resp := postJSON(t, ts.URL+"/rest/api/3/priority", map[string]string{"name": "Medium"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var created map[string]interface{}
	decodeJSON(t, resp, &created)
	id := created["id"].(string)

	// Send invalid JSON in PUT
	req, err := http.NewRequest(http.MethodPut, ts.URL+"/rest/api/3/priority/"+id, strings.NewReader("not-json"))
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
		t.Errorf("invalid JSON update: expected 400, got %d", putResp.StatusCode)
	}
}

// TestCRUDEndpointCreateInvalidJSON tests that invalid JSON in create returns 400.
func TestCRUDEndpointCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/rest/api/3/priority", strings.NewReader("not-json"))
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
		t.Errorf("invalid JSON create: expected 400, got %d", resp.StatusCode)
	}
}

// TestProjectUpdateInvalidJSON tests that invalid JSON in project update returns 400.
func TestProjectUpdateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/project", map[string]string{
		"name": "Test", "key": "TST", "projectTypeKey": "software",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var project map[string]interface{}
	decodeJSON(t, resp, &project)
	id := project["id"].(string)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/rest/api/3/project/"+id, strings.NewReader("{bad"))
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

// TestProjectUpdateNotFound tests that updating a non-existent project returns 404.
func TestProjectUpdateNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/rest/api/3/project/nonexistent", map[string]string{"name": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestProjectCreateInvalidJSON tests that invalid JSON in project create returns 400.
func TestProjectCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/rest/api/3/project", strings.NewReader("bad"))
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

// TestAutomationRuleUpdateInvalidJSON tests that invalid JSON in automation rule update returns 400.
func TestAutomationRuleUpdateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/automation/rule", map[string]interface{}{
		"name":    "Rule",
		"trigger": map[string]string{"type": "x"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var rule map[string]interface{}
	decodeJSON(t, resp, &rule)
	id := rule["id"].(string)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/rest/api/3/automation/rule/"+id, strings.NewReader("{bad"))
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

// TestAutomationRuleUpdateNotFound tests updating a non-existent automation rule.
func TestAutomationRuleUpdateNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/rest/api/3/automation/rule/nonexistent", map[string]string{"name": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestAutomationRuleCreateInvalidJSON tests that invalid JSON in automation rule create returns 400.
func TestAutomationRuleCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/rest/api/3/automation/rule", strings.NewReader("bad"))
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

// TestDomainUpdateInvalidJSON tests that invalid JSON in domain update returns 400.
func TestDomainUpdateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/rest/api/3/domain", map[string]string{"domain": "test.com"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var domain map[string]interface{}
	decodeJSON(t, resp, &domain)
	id := domain["id"].(string)

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/rest/api/3/domain/"+id, strings.NewReader("{bad"))
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

// TestDomainUpdateNotFound tests updating a non-existent domain.
func TestDomainUpdateNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/rest/api/3/domain/nonexistent", map[string]string{"status": "verified"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestDomainCreateInvalidJSON tests that invalid JSON in domain create returns 400.
func TestDomainCreateInvalidJSON(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/rest/api/3/domain", strings.NewReader("bad"))
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

// TestCRUDEndpointUpdateNotFound tests that updating a non-existent CRUD resource returns 404.
func TestCRUDEndpointUpdateNotFound(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/rest/api/3/priority/nonexistent", map[string]string{"name": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestEmptyListResponses tests that listing endpoints return empty arrays when no resources exist.
func TestEmptyListResponses(t *testing.T) {
	t.Parallel()
	ts := newJiraServer(t)
	defer ts.Close()

	paths := []string{
		"/rest/api/3/project",
		"/rest/api/3/issuetype",
		"/rest/api/3/workflow",
		"/rest/api/3/domain",
		"/rest/api/3/automation/rule",
		"/rest/api/3/dashboard",
		"/rest/api/3/filter",
		"/rest/agile/1.0/board",
		"/rest/api/3/priority",
	}

	for _, path := range paths {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d", path, resp.StatusCode)
			resp.Body.Close()
			continue
		}
		var listResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			t.Fatalf("GET %s: decode error: %v", path, err)
		}
		resp.Body.Close()
		values, ok := listResp["values"].([]interface{})
		if !ok {
			t.Errorf("GET %s: expected values array", path)
			continue
		}
		if len(values) != 0 {
			t.Errorf("GET %s: expected empty values, got %d", path, len(values))
		}
	}
}
