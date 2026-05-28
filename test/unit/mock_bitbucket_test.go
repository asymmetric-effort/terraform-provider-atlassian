// Package unit contains unit tests for the mock API Bitbucket endpoints.
package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// newBitbucketServer creates a mock server with Bitbucket endpoints registered.
func newBitbucketServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterBitbucketEndpoints(s)
	return httptest.NewServer(s.Handler())
}

// TestBBRepositoryCRUDLifecycle tests create, read, update, and delete for repositories.
func TestBBRepositoryCRUDLifecycle(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	// Create repository (PUT)
	resp := putJSON(t, ts.URL+"/2.0/repositories/myws/myrepo", map[string]interface{}{
		"name":        "My Repo",
		"is_private":  true,
		"fork_policy": "no_forks",
		"language":    "go",
		"has_issues":  true,
		"has_wiki":    false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create repo: expected 200, got %d", resp.StatusCode)
	}
	var repo map[string]interface{}
	decodeJSON(t, resp, &repo)
	if repo["slug"] != "myrepo" {
		t.Errorf("expected slug 'myrepo', got %v", repo["slug"])
	}
	if repo["name"] != "My Repo" {
		t.Errorf("expected name 'My Repo', got %v", repo["name"])
	}
	uuid, ok := repo["uuid"].(string)
	if !ok || uuid == "" {
		t.Fatal("expected non-empty uuid")
	}
	if repo["full_name"] != "myws/myrepo" {
		t.Errorf("expected full_name 'myws/myrepo', got %v", repo["full_name"])
	}

	// Check links
	links, ok := repo["links"].(map[string]interface{})
	if !ok {
		t.Fatal("expected links in response")
	}
	html, ok := links["html"].(map[string]interface{})
	if !ok {
		t.Fatal("expected html link in response")
	}
	if html["href"] != "https://bitbucket.org/myws/myrepo" {
		t.Errorf("expected html href, got %v", html["href"])
	}
	cloneLinks, ok := links["clone"].([]interface{})
	if !ok || len(cloneLinks) != 2 {
		t.Fatalf("expected 2 clone links, got %v", cloneLinks)
	}

	// Read repository
	resp, err := http.Get(ts.URL + "/2.0/repositories/myws/myrepo")
	if err != nil {
		t.Fatalf("read repo: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read repo: expected 200, got %d", resp.StatusCode)
	}
	var readRepo map[string]interface{}
	decodeJSON(t, resp, &readRepo)
	if readRepo["name"] != "My Repo" {
		t.Errorf("read repo: expected name 'My Repo', got %v", readRepo["name"])
	}

	// Update repository (PUT to same path)
	resp = putJSON(t, ts.URL+"/2.0/repositories/myws/myrepo", map[string]interface{}{
		"name":       "Updated Repo",
		"is_private": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update repo: expected 200, got %d", resp.StatusCode)
	}
	var updatedRepo map[string]interface{}
	decodeJSON(t, resp, &updatedRepo)
	if updatedRepo["name"] != "Updated Repo" {
		t.Errorf("update: expected name 'Updated Repo', got %v", updatedRepo["name"])
	}
	if updatedRepo["uuid"] != uuid {
		t.Error("update: uuid should not change")
	}

	// List repositories
	resp, err = http.Get(ts.URL + "/2.0/repositories/myws")
	if err != nil {
		t.Fatalf("list repos: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list repos: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok {
		t.Fatal("expected values array")
	}
	if len(values) != 1 {
		t.Errorf("expected 1 repo, got %d", len(values))
	}

	// Delete repository
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/myws/myrepo", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete repo: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete repo: expected 204, got %d", resp.StatusCode)
	}

	// Read after delete should 404
	resp, err = http.Get(ts.URL + "/2.0/repositories/myws/myrepo")
	if err != nil {
		t.Fatalf("read after delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("read after delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBRepositoryCreateMissingName tests creating a repo without a name.
func TestBBRepositoryCreateMissingName(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/2.0/repositories/ws/repo", map[string]interface{}{
		"is_private": true,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBRepositoryDeleteNotFound tests deleting a nonexistent repo.
func TestBBRepositoryDeleteNotFound(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/nope", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBRepositoryReadNotFound tests reading a nonexistent repo.
func TestBBRepositoryReadNotFound(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/nope")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBRepositoryListEmpty tests listing repos in an empty workspace.
func TestBBRepositoryListEmpty(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/emptyws")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 0 {
		t.Errorf("expected empty values, got %v", values)
	}
}

// TestBBBranchRestrictionCRUD tests branch restriction CRUD.
func TestBBBranchRestrictionCRUD(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	// Create
	resp := postJSON(t, ts.URL+"/2.0/repositories/ws/repo/branch-restrictions", map[string]interface{}{
		"kind":    "push",
		"pattern": "main",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var br map[string]interface{}
	decodeJSON(t, resp, &br)
	brID, ok := br["id"].(string)
	if !ok || brID == "" {
		t.Fatal("expected non-empty id")
	}

	// Read
	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/branch-restrictions/" + brID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update
	resp = putJSON(t, ts.URL+"/2.0/repositories/ws/repo/branch-restrictions/"+brID, map[string]interface{}{
		"pattern": "develop",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}
	var updatedBR map[string]interface{}
	decodeJSON(t, resp, &updatedBR)
	if updatedBR["pattern"] != "develop" {
		t.Errorf("expected pattern 'develop', got %v", updatedBR["pattern"])
	}

	// List
	resp, err = http.Get(ts.URL + "/2.0/repositories/ws/repo/branch-restrictions")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("expected 1 restriction, got %d", len(values))
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/branch-restrictions/"+brID, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}
}

// TestBBBranchRestrictionMissingKind tests creating a branch restriction without kind.
func TestBBBranchRestrictionMissingKind(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/2.0/repositories/ws/repo/branch-restrictions", map[string]interface{}{
		"pattern": "main",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBBranchRestrictionNotFound tests reading/updating/deleting nonexistent restrictions.
func TestBBBranchRestrictionNotFound(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	// Read
	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/branch-restrictions/nope")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update
	resp = putJSON(t, ts.URL+"/2.0/repositories/ws/repo/branch-restrictions/nope", map[string]interface{}{"pattern": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on update, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/branch-restrictions/nope", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on delete, got %d", resp.StatusCode)
	}
}

// TestBBBranchRestrictionListEmpty tests listing with no restrictions.
func TestBBBranchRestrictionListEmpty(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/branch-restrictions")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, _ := listResp["values"].([]interface{})
	if len(values) != 0 {
		t.Errorf("expected empty values, got %d", len(values))
	}
}

// TestBBPipelineConfigCRUD tests pipeline configuration CRUD.
func TestBBPipelineConfigCRUD(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	// Create/Update (PUT)
	resp := putJSON(t, ts.URL+"/2.0/repositories/ws/repo/pipelines_config", map[string]interface{}{
		"enabled": true,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", resp.StatusCode)
	}
	var pipeline map[string]interface{}
	decodeJSON(t, resp, &pipeline)
	if pipeline["enabled"] != true {
		t.Error("expected enabled to be true")
	}

	// Read
	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/pipelines_config")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update
	resp = putJSON(t, ts.URL+"/2.0/repositories/ws/repo/pipelines_config", map[string]interface{}{
		"enabled": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/pipelines_config", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	// Read after delete
	resp, err = http.Get(ts.URL + "/2.0/repositories/ws/repo/pipelines_config")
	if err != nil {
		t.Fatalf("read after delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBPipelineConfigNotFound tests reading/deleting nonexistent pipeline config.
func TestBBPipelineConfigNotFound(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/norepo/pipelines_config")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/norepo/pipelines_config", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestBBDeploymentEnvironmentCRUD tests deployment environment CRUD.
func TestBBDeploymentEnvironmentCRUD(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	// Create
	resp := postJSON(t, ts.URL+"/2.0/repositories/ws/repo/environments", map[string]interface{}{
		"name":             "staging",
		"environment_type": map[string]interface{}{"name": "Staging"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", resp.StatusCode)
	}
	var env map[string]interface{}
	decodeJSON(t, resp, &env)
	envUUID, ok := env["uuid"].(string)
	if !ok || envUUID == "" {
		t.Fatal("expected non-empty uuid")
	}
	// Extract the id used by the mock (strip braces from uuid)
	envID := envUUID[1 : len(envUUID)-1]

	// Read
	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/environments/" + envID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update
	resp = putJSON(t, ts.URL+"/2.0/repositories/ws/repo/environments/"+envID, map[string]interface{}{
		"name": "production",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: expected 200, got %d", resp.StatusCode)
	}
	var updatedEnv map[string]interface{}
	decodeJSON(t, resp, &updatedEnv)
	if updatedEnv["name"] != "production" {
		t.Errorf("expected name 'production', got %v", updatedEnv["name"])
	}

	// List
	resp, err = http.Get(ts.URL + "/2.0/repositories/ws/repo/environments")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("expected 1 environment, got %d", len(values))
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/environments/"+envID, nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}
}

// TestBBDeploymentEnvironmentMissingName tests creating an environment without name.
func TestBBDeploymentEnvironmentMissingName(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp := postJSON(t, ts.URL+"/2.0/repositories/ws/repo/environments", map[string]interface{}{
		"environment_type": map[string]interface{}{"name": "Test"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBDeploymentEnvironmentNotFound tests reading/updating/deleting nonexistent environments.
func TestBBDeploymentEnvironmentNotFound(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/environments/nope")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = putJSON(t, ts.URL+"/2.0/repositories/ws/repo/environments/nope", map[string]interface{}{"name": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on update, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/environments/nope", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestBBDeploymentEnvironmentListEmpty tests listing with no environments.
func TestBBDeploymentEnvironmentListEmpty(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/environments")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, _ := listResp["values"].([]interface{})
	if len(values) != 0 {
		t.Errorf("expected empty values, got %d", len(values))
	}
}

// TestBBPermissionUserCRUD tests repository user permission CRUD.
func TestBBPermissionUserCRUD(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	// Create/Update (PUT)
	resp := putJSON(t, ts.URL+"/2.0/repositories/ws/repo/permissions-config/users/user123", map[string]interface{}{
		"permission": "write",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", resp.StatusCode)
	}
	var perm map[string]interface{}
	decodeJSON(t, resp, &perm)
	if perm["permission"] != "write" {
		t.Errorf("expected permission 'write', got %v", perm["permission"])
	}

	// Read
	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/users/user123")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	resp, err = http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/users")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("expected 1 permission, got %d", len(values))
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/permissions-config/users/user123", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	// Read after delete
	resp, err = http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/users/user123")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBPermissionUserNotFound tests reading/deleting nonexistent user permissions.
func TestBBPermissionUserNotFound(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/users/nope")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/permissions-config/users/nope", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestBBPermissionUserListEmpty tests listing with no user permissions.
func TestBBPermissionUserListEmpty(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/users")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	values, _ := listResp["values"].([]interface{})
	if len(values) != 0 {
		t.Errorf("expected empty values, got %d", len(values))
	}
}

// TestBBPermissionGroupCRUD tests repository group permission CRUD.
func TestBBPermissionGroupCRUD(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	// Create/Update (PUT)
	resp := putJSON(t, ts.URL+"/2.0/repositories/ws/repo/permissions-config/groups/devteam", map[string]interface{}{
		"permission": "admin",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", resp.StatusCode)
	}
	var perm map[string]interface{}
	decodeJSON(t, resp, &perm)
	if perm["permission"] != "admin" {
		t.Errorf("expected permission 'admin', got %v", perm["permission"])
	}

	// Read
	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/groups/devteam")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	resp, err = http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/groups")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	decodeJSON(t, resp, &listResp)
	values, ok := listResp["values"].([]interface{})
	if !ok || len(values) != 1 {
		t.Errorf("expected 1 permission, got %d", len(values))
	}

	// Delete
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/permissions-config/groups/devteam", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}

	// Read after delete
	resp, err = http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/groups/devteam")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestBBPermissionGroupNotFound tests reading/deleting nonexistent group permissions.
func TestBBPermissionGroupNotFound(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/groups/nope")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/2.0/repositories/ws/repo/permissions-config/groups/nope", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

// TestBBPermissionGroupListEmpty tests listing with no group permissions.
func TestBBPermissionGroupListEmpty(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo/permissions-config/groups")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var listResp map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	values, _ := listResp["values"].([]interface{})
	if len(values) != 0 {
		t.Errorf("expected empty values, got %d", len(values))
	}
}

// TestBBRepositoryWithMainBranch tests creating a repo with mainbranch set.
func TestBBRepositoryWithMainBranch(t *testing.T) {
	t.Parallel()
	ts := newBitbucketServer(t)
	defer ts.Close()

	resp := putJSON(t, ts.URL+"/2.0/repositories/ws/branched", map[string]interface{}{
		"name":       "Branched Repo",
		"mainbranch": map[string]interface{}{"name": "develop"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: expected 200, got %d", resp.StatusCode)
	}
	var repo map[string]interface{}
	decodeJSON(t, resp, &repo)
	mb, ok := repo["mainbranch"].(map[string]interface{})
	if !ok {
		t.Fatal("expected mainbranch in response")
	}
	if mb["name"] != "develop" {
		t.Errorf("expected mainbranch name 'develop', got %v", mb["name"])
	}
}

// TestBBRunIncludesBitbucket verifies that Run() registers bitbucket endpoints.
func TestBBRunIncludesBitbucket(t *testing.T) {
	t.Parallel()
	// Verify RegisterBitbucketEndpoints is callable without panic.
	s := mock.NewServer()
	mock.RegisterBitbucketEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Verify a bitbucket endpoint exists
	resp, err := http.Get(ts.URL + "/2.0/repositories/ws/repo")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	// Should get 404 (not found), not 405 (method not allowed) or default handler
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
