// Package integration contains comprehensive integration tests for Bitbucket
// resources exercised against the mock API server.
//
// These tests verify full CRUD lifecycles, cross-resource operations,
// import patterns, idempotency, drift detection, and error handling for:
// repository, branch_restriction, pipeline, deployment, repository_permission.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// setupBitbucketMockServer creates a mock server with auth, identity, and
// Bitbucket endpoints, and returns the httptest server and a configured client.
func setupBitbucketMockServer(t *testing.T) (*httptest.Server, *client.Client) {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	mock.RegisterIdentityEndpoints(s)
	mock.RegisterBitbucketEndpoints(s)
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

// bbBody marshals v to a bytes.Reader for use as a request body.
func bbBody(t *testing.T, v interface{}) *bytes.Reader {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal body: %v", err)
	}
	return bytes.NewReader(data)
}

// ============================================================================
// Repository CRUD Lifecycle
// ============================================================================

func TestBitbucketIntegrationRepositoryCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	workspace := "testws"
	slug := "my-repo"
	basePath := fmt.Sprintf("/2.0/repositories/%s/%s", workspace, slug)

	// Create
	createBody := map[string]interface{}{
		"scm":        "git",
		"is_private": true,
		"name":       "My Repository",
	}
	var created map[string]interface{}
	err := c.Post(ctx, basePath, bbBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create repository failed: %v", err)
	}
	if created["slug"] != slug {
		t.Errorf("create: expected slug %q, got %v", slug, created["slug"])
	}
	if created["full_name"] != workspace+"/"+slug {
		t.Errorf("create: expected full_name %q, got %v", workspace+"/"+slug, created["full_name"])
	}
	if created["scm"] != "git" {
		t.Errorf("create: expected scm 'git', got %v", created["scm"])
	}
	if created["is_private"] != true {
		t.Errorf("create: expected is_private true, got %v", created["is_private"])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, basePath, &read)
	if err != nil {
		t.Fatalf("read repository failed: %v", err)
	}
	if read["slug"] != slug {
		t.Errorf("read: expected slug %q, got %v", slug, read["slug"])
	}
	if read["name"] != "My Repository" {
		t.Errorf("read: expected name 'My Repository', got %v", read["name"])
	}

	// Update
	updateBody := map[string]interface{}{
		"description": "Updated description",
		"is_private":  false,
	}
	var updated map[string]interface{}
	err = c.Put(ctx, basePath, bbBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update repository failed: %v", err)
	}
	if updated["description"] != "Updated description" {
		t.Errorf("update: expected description 'Updated description', got %v", updated["description"])
	}
	if updated["is_private"] != false {
		t.Errorf("update: expected is_private false, got %v", updated["is_private"])
	}
	// Immutable fields preserved
	if updated["slug"] != slug {
		t.Errorf("update: slug should be immutable, got %v", updated["slug"])
	}

	// Re-read to verify persistence
	var reread map[string]interface{}
	err = c.Get(ctx, basePath, &reread)
	if err != nil {
		t.Fatalf("re-read repository failed: %v", err)
	}
	if reread["description"] != "Updated description" {
		t.Errorf("re-read: update not persisted, expected 'Updated description', got %v", reread["description"])
	}

	// Delete
	err = c.Delete(ctx, basePath)
	if err != nil {
		t.Fatalf("delete repository failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, basePath, &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted repository, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404 for deleted repository, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Repository: duplicate slug
// ============================================================================

func TestBitbucketIntegrationRepositoryDuplicateSlug(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	basePath := "/2.0/repositories/testws/dup-repo"
	body := map[string]interface{}{"scm": "git"}

	err := c.Post(ctx, basePath, bbBody(t, body), nil)
	if err != nil {
		t.Fatalf("create first repository failed: %v", err)
	}

	// Duplicate slug should fail with 409
	err = c.Post(ctx, basePath, bbBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for duplicate slug, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected 409 for duplicate slug, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Branch Restriction CRUD Lifecycle
// ============================================================================

func TestBitbucketIntegrationBranchRestrictionCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	repoBase := "/2.0/repositories/testws/br-repo"
	restrictionBase := repoBase + "/branch-restrictions"

	// Create the repository first
	err := c.Post(ctx, repoBase, bbBody(t, map[string]interface{}{"scm": "git"}), nil)
	if err != nil {
		t.Fatalf("create repository failed: %v", err)
	}

	// Create branch restriction
	createBody := map[string]interface{}{
		"kind":    "push",
		"pattern": "main",
	}
	var created map[string]interface{}
	err = c.Post(ctx, restrictionBase, bbBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create branch restriction failed: %v", err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatal("create: expected non-empty id")
	}
	if created["kind"] != "push" {
		t.Errorf("create: expected kind 'push', got %v", created["kind"])
	}
	if created["pattern"] != "main" {
		t.Errorf("create: expected pattern 'main', got %v", created["pattern"])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", restrictionBase, id), &read)
	if err != nil {
		t.Fatalf("read branch restriction failed: %v", err)
	}
	if read["kind"] != "push" {
		t.Errorf("read: expected kind 'push', got %v", read["kind"])
	}

	// Update
	updateBody := map[string]interface{}{
		"pattern": "develop",
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("%s/%s", restrictionBase, id), bbBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update branch restriction failed: %v", err)
	}
	if updated["pattern"] != "develop" {
		t.Errorf("update: expected pattern 'develop', got %v", updated["pattern"])
	}
	if updated["id"] != id {
		t.Errorf("update: id should not change, got %v", updated["id"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("%s/%s", restrictionBase, id))
	if err != nil {
		t.Fatalf("delete branch restriction failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", restrictionBase, id), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted branch restriction, got nil")
	}
}

// ============================================================================
// Branch Restriction: invalid pattern
// ============================================================================

func TestBitbucketIntegrationBranchRestrictionInvalidPattern(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	restrictionBase := "/2.0/repositories/testws/pattern-repo/branch-restrictions"

	body := map[string]interface{}{
		"kind":    "push",
		"pattern": "{invalid}",
	}
	err := c.Post(ctx, restrictionBase, bbBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for invalid branch pattern, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400 for invalid pattern, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Pipeline Configuration CRUD Lifecycle
// ============================================================================

func TestBitbucketIntegrationPipelineCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	repoBase := "/2.0/repositories/testws/pipe-repo"
	pipelinePath := repoBase + "/pipelines_config"

	// Create the repository first
	err := c.Post(ctx, repoBase, bbBody(t, map[string]interface{}{"scm": "git"}), nil)
	if err != nil {
		t.Fatalf("create repository failed: %v", err)
	}

	// Read default (should return disabled)
	var defaultConfig map[string]interface{}
	err = c.Get(ctx, pipelinePath, &defaultConfig)
	if err != nil {
		t.Fatalf("read default pipeline config failed: %v", err)
	}
	if defaultConfig["enabled"] != false {
		t.Errorf("default pipeline config should be disabled, got %v", defaultConfig["enabled"])
	}

	// Enable pipeline (PUT creates/updates)
	enableBody := map[string]interface{}{
		"enabled": true,
	}
	var enabled map[string]interface{}
	err = c.Put(ctx, pipelinePath, bbBody(t, enableBody), &enabled)
	if err != nil {
		t.Fatalf("enable pipeline failed: %v", err)
	}
	if enabled["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", enabled["enabled"])
	}

	// Re-read to verify persistence
	var reread map[string]interface{}
	err = c.Get(ctx, pipelinePath, &reread)
	if err != nil {
		t.Fatalf("re-read pipeline config failed: %v", err)
	}
	if reread["enabled"] != true {
		t.Errorf("re-read: expected enabled=true, got %v", reread["enabled"])
	}

	// Update: disable pipeline
	disableBody := map[string]interface{}{
		"enabled": false,
	}
	var disabled map[string]interface{}
	err = c.Put(ctx, pipelinePath, bbBody(t, disableBody), &disabled)
	if err != nil {
		t.Fatalf("disable pipeline failed: %v", err)
	}
	if disabled["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", disabled["enabled"])
	}

	// Delete pipeline config
	err = c.Delete(ctx, pipelinePath)
	if err != nil {
		t.Fatalf("delete pipeline config failed: %v", err)
	}
}

// ============================================================================
// Pipeline: config validation errors
// ============================================================================

func TestBitbucketIntegrationPipelineConfigMissingEnabled(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pipelinePath := "/2.0/repositories/testws/pipe-err-repo/pipelines_config"

	err := c.Put(ctx, pipelinePath, bbBody(t, map[string]interface{}{}), nil)
	if err == nil {
		t.Fatal("expected error for missing enabled field, got nil")
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
// Deployment Environment CRUD Lifecycle
// ============================================================================

func TestBitbucketIntegrationDeploymentCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	repoBase := "/2.0/repositories/testws/deploy-repo"
	envBase := repoBase + "/environments"

	// Create the repository first
	err := c.Post(ctx, repoBase, bbBody(t, map[string]interface{}{"scm": "git"}), nil)
	if err != nil {
		t.Fatalf("create repository failed: %v", err)
	}

	// Create deployment environment
	createBody := map[string]interface{}{
		"name": "Production",
		"environment_type": map[string]interface{}{
			"name": "Production",
			"rank": 2,
		},
	}
	var created map[string]interface{}
	err = c.Post(ctx, envBase, bbBody(t, createBody), &created)
	if err != nil {
		t.Fatalf("create deployment failed: %v", err)
	}
	id, ok := created["uuid"].(string)
	if !ok || id == "" {
		t.Fatal("create: expected non-empty uuid")
	}
	if created["name"] != "Production" {
		t.Errorf("create: expected name 'Production', got %v", created["name"])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", envBase, id), &read)
	if err != nil {
		t.Fatalf("read deployment failed: %v", err)
	}
	if read["name"] != "Production" {
		t.Errorf("read: expected name 'Production', got %v", read["name"])
	}

	// Update
	updateBody := map[string]interface{}{
		"name": "Staging",
	}
	var updated map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("%s/%s", envBase, id), bbBody(t, updateBody), &updated)
	if err != nil {
		t.Fatalf("update deployment failed: %v", err)
	}
	if updated["name"] != "Staging" {
		t.Errorf("update: expected name 'Staging', got %v", updated["name"])
	}

	// Delete
	err = c.Delete(ctx, fmt.Sprintf("%s/%s", envBase, id))
	if err != nil {
		t.Fatalf("delete deployment failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", envBase, id), &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted deployment, got nil")
	}
}

// ============================================================================
// Deployment: duplicate name in same repo
// ============================================================================

func TestBitbucketIntegrationDeploymentDuplicateName(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	envBase := "/2.0/repositories/testws/dup-deploy-repo/environments"
	body := map[string]interface{}{
		"name": "Production",
		"environment_type": map[string]interface{}{
			"name": "Production",
			"rank": 2,
		},
	}

	err := c.Post(ctx, envBase, bbBody(t, body), nil)
	if err != nil {
		t.Fatalf("create first deployment failed: %v", err)
	}

	err = c.Post(ctx, envBase, bbBody(t, body), nil)
	if err == nil {
		t.Fatal("expected error for duplicate deployment name, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected 409 for duplicate deployment, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Repository Permission CRUD Lifecycle
// ============================================================================

func TestBitbucketIntegrationRepositoryPermissionCRUDLifecycle(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	repoBase := "/2.0/repositories/testws/perm-repo"
	userID := "user-abc-123"
	permPath := fmt.Sprintf("%s/permissions-config/users/%s", repoBase, userID)

	// Create the repository first
	err := c.Post(ctx, repoBase, bbBody(t, map[string]interface{}{"scm": "git"}), nil)
	if err != nil {
		t.Fatalf("create repository failed: %v", err)
	}

	// Set user permission (PUT = create/update)
	var created map[string]interface{}
	err = c.Put(ctx, permPath, bbBody(t, map[string]interface{}{"permission": "write"}), &created)
	if err != nil {
		t.Fatalf("set permission failed: %v", err)
	}
	if created["permission"] != "write" {
		t.Errorf("create: expected permission 'write', got %v", created["permission"])
	}

	// Read
	var read map[string]interface{}
	err = c.Get(ctx, permPath, &read)
	if err != nil {
		t.Fatalf("read permission failed: %v", err)
	}
	if read["permission"] != "write" {
		t.Errorf("read: expected permission 'write', got %v", read["permission"])
	}

	// Update to admin
	var updated map[string]interface{}
	err = c.Put(ctx, permPath, bbBody(t, map[string]interface{}{"permission": "admin"}), &updated)
	if err != nil {
		t.Fatalf("update permission failed: %v", err)
	}
	if updated["permission"] != "admin" {
		t.Errorf("update: expected permission 'admin', got %v", updated["permission"])
	}

	// Re-read to verify
	var reread map[string]interface{}
	err = c.Get(ctx, permPath, &reread)
	if err != nil {
		t.Fatalf("re-read permission failed: %v", err)
	}
	if reread["permission"] != "admin" {
		t.Errorf("re-read: expected permission 'admin', got %v", reread["permission"])
	}

	// Delete
	err = c.Delete(ctx, permPath)
	if err != nil {
		t.Fatalf("delete permission failed: %v", err)
	}

	// Verify gone
	var ghost map[string]interface{}
	err = c.Get(ctx, permPath, &ghost)
	if err == nil {
		t.Fatal("expected error reading deleted permission, got nil")
	}
}

// ============================================================================
// Repository Permission: invalid permission value
// ============================================================================

func TestBitbucketIntegrationPermissionInvalidValue(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	permPath := "/2.0/repositories/testws/inv-perm-repo/permissions-config/users/user1"

	err := c.Put(ctx, permPath, bbBody(t, map[string]interface{}{"permission": "execute"}), nil)
	if err == nil {
		t.Fatal("expected error for invalid permission value, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400 for invalid permission, got %d", apiErr.StatusCode)
	}
}

// ============================================================================
// Cross-Resource: repo -> branch restriction -> pipeline -> deployment -> permissions
// ============================================================================

func TestBitbucketIntegrationCrossResourceFullStack(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	workspace := "crossws"
	slug := "full-stack-repo"
	repoBase := fmt.Sprintf("/2.0/repositories/%s/%s", workspace, slug)

	// Step 1: Create repository
	var repo map[string]interface{}
	err := c.Post(ctx, repoBase, bbBody(t, map[string]interface{}{
		"scm":        "git",
		"is_private": true,
		"name":       "Full Stack Repo",
	}), &repo)
	if err != nil {
		t.Fatalf("create repository failed: %v", err)
	}
	t.Logf("created repository: %s/%s", workspace, slug)

	// Step 2: Add branch restriction
	var restriction map[string]interface{}
	err = c.Post(ctx, repoBase+"/branch-restrictions", bbBody(t, map[string]interface{}{
		"kind":    "push",
		"pattern": "main",
	}), &restriction)
	if err != nil {
		t.Fatalf("create branch restriction failed: %v", err)
	}
	restrictionID := restriction["id"].(string)
	t.Logf("created branch restriction: %s (kind=push, pattern=main)", restrictionID)

	// Step 3: Configure pipeline
	var pipeline map[string]interface{}
	err = c.Put(ctx, repoBase+"/pipelines_config", bbBody(t, map[string]interface{}{
		"enabled": true,
	}), &pipeline)
	if err != nil {
		t.Fatalf("enable pipeline failed: %v", err)
	}
	if pipeline["enabled"] != true {
		t.Error("pipeline should be enabled")
	}
	t.Logf("configured pipeline: enabled=true")

	// Step 4: Add deployment environment
	var deployment map[string]interface{}
	err = c.Post(ctx, repoBase+"/environments", bbBody(t, map[string]interface{}{
		"name": "Production",
		"environment_type": map[string]interface{}{
			"name": "Production",
			"rank": 2,
		},
	}), &deployment)
	if err != nil {
		t.Fatalf("create deployment failed: %v", err)
	}
	deploymentID := deployment["uuid"].(string)
	t.Logf("created deployment: %s (name=Production)", deploymentID)

	// Step 5: Set permissions
	userID := "user-dev-001"
	permPath := fmt.Sprintf("%s/permissions-config/users/%s", repoBase, userID)
	var perm map[string]interface{}
	err = c.Put(ctx, permPath, bbBody(t, map[string]interface{}{
		"permission": "write",
	}), &perm)
	if err != nil {
		t.Fatalf("set permission failed: %v", err)
	}
	t.Logf("set permission: user=%s, permission=write", userID)

	// Verify all resources are readable
	var readRepo map[string]interface{}
	err = c.Get(ctx, repoBase, &readRepo)
	if err != nil {
		t.Fatalf("cross-resource: read repository failed: %v", err)
	}

	var readRestriction map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/branch-restrictions/%s", repoBase, restrictionID), &readRestriction)
	if err != nil {
		t.Fatalf("cross-resource: read branch restriction failed: %v", err)
	}

	var readPipeline map[string]interface{}
	err = c.Get(ctx, repoBase+"/pipelines_config", &readPipeline)
	if err != nil {
		t.Fatalf("cross-resource: read pipeline config failed: %v", err)
	}

	var readDeployment map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/environments/%s", repoBase, deploymentID), &readDeployment)
	if err != nil {
		t.Fatalf("cross-resource: read deployment failed: %v", err)
	}

	var readPerm map[string]interface{}
	err = c.Get(ctx, permPath, &readPerm)
	if err != nil {
		t.Fatalf("cross-resource: read permission failed: %v", err)
	}

	// Cleanup in reverse order
	err = c.Delete(ctx, permPath)
	if err != nil {
		t.Fatalf("cleanup: delete permission failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("%s/environments/%s", repoBase, deploymentID))
	if err != nil {
		t.Fatalf("cleanup: delete deployment failed: %v", err)
	}
	err = c.Delete(ctx, repoBase+"/pipelines_config")
	if err != nil {
		t.Fatalf("cleanup: delete pipeline config failed: %v", err)
	}
	err = c.Delete(ctx, fmt.Sprintf("%s/branch-restrictions/%s", repoBase, restrictionID))
	if err != nil {
		t.Fatalf("cleanup: delete branch restriction failed: %v", err)
	}
	err = c.Delete(ctx, repoBase)
	if err != nil {
		t.Fatalf("cleanup: delete repository failed: %v", err)
	}
}

// ============================================================================
// Import: read-by-ID patterns
// ============================================================================

func TestBitbucketIntegrationImportRepositoryBySlug(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	basePath := "/2.0/repositories/importws/import-repo"
	err := c.Post(ctx, basePath, bbBody(t, map[string]interface{}{"scm": "git", "name": "Import Repo"}), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	var imported map[string]interface{}
	err = c.Get(ctx, basePath, &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["name"] != "Import Repo" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
	if imported["full_name"] != "importws/import-repo" {
		t.Errorf("import: full_name mismatch: got %v", imported["full_name"])
	}
}

func TestBitbucketIntegrationImportBranchRestrictionByID(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	restrictionBase := "/2.0/repositories/importws/import-br-repo/branch-restrictions"
	var created map[string]interface{}
	err := c.Post(ctx, restrictionBase, bbBody(t, map[string]interface{}{
		"kind":    "push",
		"pattern": "release/*",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", restrictionBase, id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["kind"] != "push" {
		t.Errorf("import: kind mismatch: got %v", imported["kind"])
	}
	if imported["pattern"] != "release/*" {
		t.Errorf("import: pattern mismatch: got %v", imported["pattern"])
	}
}

func TestBitbucketIntegrationImportDeploymentByID(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	envBase := "/2.0/repositories/importws/import-deploy-repo/environments"
	var created map[string]interface{}
	err := c.Post(ctx, envBase, bbBody(t, map[string]interface{}{
		"name": "Staging",
		"environment_type": map[string]interface{}{
			"name": "Staging",
			"rank": 1,
		},
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["uuid"].(string)

	var imported map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", envBase, id), &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["name"] != "Staging" {
		t.Errorf("import: name mismatch: got %v", imported["name"])
	}
}

func TestBitbucketIntegrationImportPermissionByUserID(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	userID := "import-user-1"
	permPath := fmt.Sprintf("/2.0/repositories/importws/import-perm-repo/permissions-config/users/%s", userID)

	err := c.Put(ctx, permPath, bbBody(t, map[string]interface{}{"permission": "read"}), nil)
	if err != nil {
		t.Fatalf("set permission failed: %v", err)
	}

	var imported map[string]interface{}
	err = c.Get(ctx, permPath, &imported)
	if err != nil {
		t.Fatalf("import read failed: %v", err)
	}
	if imported["permission"] != "read" {
		t.Errorf("import: permission mismatch: got %v", imported["permission"])
	}
}

// ============================================================================
// Idempotency Tests
// ============================================================================

func TestBitbucketIntegrationRepositoryUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	basePath := "/2.0/repositories/idempws/idemp-repo"
	err := c.Post(ctx, basePath, bbBody(t, map[string]interface{}{
		"scm":         "git",
		"description": "Idempotent repo",
	}), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	updateBody := map[string]interface{}{"description": "Same value"}
	var first, second map[string]interface{}

	err = c.Put(ctx, basePath, bbBody(t, updateBody), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, basePath, bbBody(t, updateBody), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["description"] != second["description"] {
		t.Errorf("idempotency: description differs: %v vs %v", first["description"], second["description"])
	}
	if first["slug"] != second["slug"] {
		t.Errorf("idempotency: slug differs: %v vs %v", first["slug"], second["slug"])
	}
}

func TestBitbucketIntegrationPermissionUpdateIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	permPath := "/2.0/repositories/idempws/idemp-perm-repo/permissions-config/users/user-idemp"
	body := map[string]interface{}{"permission": "write"}

	var first, second map[string]interface{}
	err := c.Put(ctx, permPath, bbBody(t, body), &first)
	if err != nil {
		t.Fatalf("first set failed: %v", err)
	}
	err = c.Put(ctx, permPath, bbBody(t, body), &second)
	if err != nil {
		t.Fatalf("second set failed: %v", err)
	}

	if first["permission"] != second["permission"] {
		t.Errorf("idempotency: permission differs: %v vs %v", first["permission"], second["permission"])
	}
}

func TestBitbucketIntegrationPipelineConfigIdempotency(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pipelinePath := "/2.0/repositories/idempws/idemp-pipe-repo/pipelines_config"
	body := map[string]interface{}{"enabled": true}

	var first, second map[string]interface{}
	err := c.Put(ctx, pipelinePath, bbBody(t, body), &first)
	if err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	err = c.Put(ctx, pipelinePath, bbBody(t, body), &second)
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}

	if first["enabled"] != second["enabled"] {
		t.Errorf("idempotency: enabled differs: %v vs %v", first["enabled"], second["enabled"])
	}
}

// ============================================================================
// Drift Detection
// ============================================================================

func TestBitbucketIntegrationRepositoryDriftDetection(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	basePath := "/2.0/repositories/driftws/drift-repo"

	// Create with known state
	err := c.Post(ctx, basePath, bbBody(t, map[string]interface{}{
		"scm":         "git",
		"description": "Original description",
	}), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Simulate external modification (direct PUT, as if someone changed it outside OpenTofu)
	var externally map[string]interface{}
	err = c.Put(ctx, basePath, bbBody(t, map[string]interface{}{
		"description": "Modified externally",
	}), &externally)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read should detect drift
	var current map[string]interface{}
	err = c.Get(ctx, basePath, &current)
	if err != nil {
		t.Fatalf("drift read failed: %v", err)
	}

	// The current state should reflect the external modification
	if current["description"] != "Modified externally" {
		t.Errorf("drift detection: expected 'Modified externally', got %v", current["description"])
	}
}

func TestBitbucketIntegrationBranchRestrictionDriftDetection(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	restrictionBase := "/2.0/repositories/driftws/drift-br-repo/branch-restrictions"

	var created map[string]interface{}
	err := c.Post(ctx, restrictionBase, bbBody(t, map[string]interface{}{
		"kind":    "push",
		"pattern": "main",
	}), &created)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	id := created["id"].(string)

	// External modification
	var externally map[string]interface{}
	err = c.Put(ctx, fmt.Sprintf("%s/%s", restrictionBase, id), bbBody(t, map[string]interface{}{
		"pattern": "develop",
	}), &externally)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read detects drift
	var current map[string]interface{}
	err = c.Get(ctx, fmt.Sprintf("%s/%s", restrictionBase, id), &current)
	if err != nil {
		t.Fatalf("drift read failed: %v", err)
	}
	if current["pattern"] != "develop" {
		t.Errorf("drift detection: expected pattern 'develop', got %v", current["pattern"])
	}
}

func TestBitbucketIntegrationPipelineDriftDetection(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pipelinePath := "/2.0/repositories/driftws/drift-pipe-repo/pipelines_config"

	// Set enabled=true
	err := c.Put(ctx, pipelinePath, bbBody(t, map[string]interface{}{"enabled": true}), nil)
	if err != nil {
		t.Fatalf("enable pipeline failed: %v", err)
	}

	// External modification: disable
	err = c.Put(ctx, pipelinePath, bbBody(t, map[string]interface{}{"enabled": false}), nil)
	if err != nil {
		t.Fatalf("external modification failed: %v", err)
	}

	// Read detects drift
	var current map[string]interface{}
	err = c.Get(ctx, pipelinePath, &current)
	if err != nil {
		t.Fatalf("drift read failed: %v", err)
	}
	if current["enabled"] != false {
		t.Errorf("drift detection: expected enabled=false, got %v", current["enabled"])
	}
}

// ============================================================================
// Error Handling
// ============================================================================

func TestBitbucketIntegrationRepositoryNotFoundReturns404(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/nows/norepo", &result)
	if err == nil {
		t.Fatal("expected error for non-existent repository, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationBranchRestrictionNotFoundReturns404(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/testws/repo/branch-restrictions/nonexistent", &result)
	if err == nil {
		t.Fatal("expected error for non-existent branch restriction, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationDeploymentNotFoundReturns404(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/testws/repo/environments/nonexistent", &result)
	if err == nil {
		t.Fatal("expected error for non-existent deployment, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationPermissionNotFoundReturns404(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/testws/repo/permissions-config/users/nonexistent", &result)
	if err == nil {
		t.Fatal("expected error for non-existent permission, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationBranchRestrictionMissingKind(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Post(ctx, "/2.0/repositories/testws/repo/branch-restrictions", bbBody(t, map[string]interface{}{
		"pattern": "main",
	}), nil)
	if err == nil {
		t.Fatal("expected error for missing kind, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationBranchRestrictionMissingPattern(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Post(ctx, "/2.0/repositories/testws/repo/branch-restrictions", bbBody(t, map[string]interface{}{
		"kind": "push",
	}), nil)
	if err == nil {
		t.Fatal("expected error for missing pattern, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationDeploymentMissingName(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Post(ctx, "/2.0/repositories/testws/repo/environments", bbBody(t, map[string]interface{}{
		"environment_type": map[string]interface{}{"name": "Production"},
	}), nil)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationDeploymentMissingEnvironmentType(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Post(ctx, "/2.0/repositories/testws/repo/environments", bbBody(t, map[string]interface{}{
		"name": "Production",
	}), nil)
	if err == nil {
		t.Fatal("expected error for missing environment_type, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationPermissionMissingPermission(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Put(ctx, "/2.0/repositories/testws/repo/permissions-config/users/user1", bbBody(t, map[string]interface{}{}), nil)
	if err == nil {
		t.Fatal("expected error for missing permission, got nil")
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
// Delete non-existent resources returns 404
// ============================================================================

func TestBitbucketIntegrationDeleteNonExistentRepository(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/2.0/repositories/testws/nonexistent")
	if err == nil {
		t.Fatal("expected error deleting non-existent repository, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationDeleteNonExistentBranchRestriction(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/2.0/repositories/testws/repo/branch-restrictions/nonexistent")
	if err == nil {
		t.Fatal("expected error deleting non-existent branch restriction, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationDeleteNonExistentDeployment(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/2.0/repositories/testws/repo/environments/nonexistent")
	if err == nil {
		t.Fatal("expected error deleting non-existent deployment, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketIntegrationDeleteNonExistentPermission(t *testing.T) {
	t.Parallel()
	_, c := setupBitbucketMockServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	err := c.Delete(ctx, "/2.0/repositories/testws/repo/permissions-config/users/nonexistent")
	if err == nil {
		t.Fatal("expected error deleting non-existent permission, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected 404, got %d", apiErr.StatusCode)
	}
}

// suppress unused import warning
var _ = strings.Contains
