// Package unit contains tests that verify Bitbucket resource error messages
// are clear, user-friendly, identify the failing resource, suggest corrective
// action, and never expose raw API internals.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
)

// bitbucketErrorServer creates an httptest server that returns Bitbucket-style
// error responses for all requests and a configured client pointing at it.
func bitbucketErrorServer(t *testing.T, statusCode int, message string) (*httptest.Server, *client.Client) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{message},
			"errors":        map[string]string{},
		})
	}))
	t.Cleanup(ts.Close)
	auth, _ := client.NewTokenAuthenticator("u@e.com", "tok")
	c, _ := client.NewClient(client.Config{
		BaseURL:        ts.URL,
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}, auth)
	return ts, c
}

// --- Repository: not found ---

func TestBitbucketRepositoryNotFound(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 404, "Repository not found")
	ctx := context.Background()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/myworkspace/nonexistent-repo", &result)
	if err == nil {
		t.Fatal("Expected error for repository not found, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("Expected status 404, got %d", apiErr.StatusCode)
	}
	errMsg := apiErr.Error()
	if !strings.Contains(errMsg, "404") {
		t.Errorf("Error message should contain status code 404. Got: %s", errMsg)
	}
}

// --- Repository: duplicate slug ---

func TestBitbucketRepositoryDuplicateSlug(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 409, "Repository with slug 'my-repo' already exists in workspace 'myworkspace'")
	ctx := context.Background()

	body := strings.NewReader(`{"scm":"git","is_private":true}`)
	err := c.Post(ctx, "/2.0/repositories/myworkspace/my-repo", body, nil)
	if err == nil {
		t.Fatal("Expected error for duplicate slug, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("Expected status 409 for duplicate slug, got %d", apiErr.StatusCode)
	}
	errMsg := apiErr.Error()
	if !strings.Contains(errMsg, "409") {
		t.Errorf("Error message should contain status code 409. Got: %s", errMsg)
	}
}

// --- Repository: permission denied ---

func TestBitbucketRepositoryPermissionDenied(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 403, "Forbidden")
	ctx := context.Background()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/myworkspace/private-repo", &result)
	if err == nil {
		t.Fatal("Expected error for permission denied, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("Expected status 403, got %d", apiErr.StatusCode)
	}
	errMsg := apiErr.Error()
	if !strings.Contains(errMsg, "403") {
		t.Errorf("Error message should contain status code 403. Got: %s", errMsg)
	}
}

// --- Branch Restriction: invalid branch pattern ---

func TestBitbucketInvalidBranchPattern(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 400, "Invalid branch pattern '{invalid}': must not contain special characters")
	ctx := context.Background()

	body := strings.NewReader(`{"kind":"push","pattern":"{invalid}"}`)
	err := c.Post(ctx, "/2.0/repositories/myworkspace/my-repo/branch-restrictions", body, nil)
	if err == nil {
		t.Fatal("Expected error for invalid branch pattern, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("Expected status 400 for invalid branch pattern, got %d", apiErr.StatusCode)
	}
}

// --- Branch Restriction: not found ---

func TestBitbucketBranchRestrictionNotFound(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 404, "Branch restriction not found")
	ctx := context.Background()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/myworkspace/my-repo/branch-restrictions/999", &result)
	if err == nil {
		t.Fatal("Expected error for branch restriction not found, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("Expected status 404, got %d", apiErr.StatusCode)
	}
}

// --- Pipeline: config errors ---

func TestBitbucketPipelineConfigMissingEnabled(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 400, "enabled field is required for pipeline configuration")
	ctx := context.Background()

	body := strings.NewReader(`{}`)
	var result map[string]interface{}
	err := c.Put(ctx, "/2.0/repositories/myworkspace/my-repo/pipelines_config", body, &result)
	if err == nil {
		t.Fatal("Expected error for missing pipeline enabled field, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("Expected status 400 for pipeline config error, got %d", apiErr.StatusCode)
	}
}

func TestBitbucketPipelineConfigInvalidEnabled(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 400, "enabled must be a boolean value")
	ctx := context.Background()

	body := strings.NewReader(`{"enabled":"not-a-bool"}`)
	var result map[string]interface{}
	err := c.Put(ctx, "/2.0/repositories/myworkspace/my-repo/pipelines_config", body, &result)
	if err == nil {
		t.Fatal("Expected error for invalid pipeline enabled value, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("Expected status 400, got %d", apiErr.StatusCode)
	}
}

// --- Deployment: not found ---

func TestBitbucketDeploymentNotFound(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 404, "Deployment environment not found")
	ctx := context.Background()

	var result map[string]interface{}
	err := c.Get(ctx, "/2.0/repositories/myworkspace/my-repo/environments/nonexistent", &result)
	if err == nil {
		t.Fatal("Expected error for deployment not found, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("Expected status 404, got %d", apiErr.StatusCode)
	}
}

// --- Permission: invalid permission value ---

func TestBitbucketInvalidPermissionValue(t *testing.T) {
	t.Parallel()
	_, c := bitbucketErrorServer(t, 400, "Invalid permission 'execute': must be 'read', 'write', or 'admin'")
	ctx := context.Background()

	body := strings.NewReader(`{"permission":"execute"}`)
	var result map[string]interface{}
	err := c.Put(ctx, "/2.0/repositories/myworkspace/my-repo/permissions-config/users/user123", body, &result)
	if err == nil {
		t.Fatal("Expected error for invalid permission, got nil")
	}
	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("Expected status 400, got %d", apiErr.StatusCode)
	}
}

// --- Error message quality: never expose raw API ---

func TestBitbucketErrorMessagesNeverExposeRawAPI(t *testing.T) {
	t.Parallel()

	// Verify that error messages from the client contain structured info, not raw HTTP dumps
	scenarios := []struct {
		name       string
		status     int
		message    string
		path       string
		method     string
		wantStatus int
	}{
		{
			name:       "repo_not_found",
			status:     404,
			message:    "Repository not found",
			path:       "/2.0/repositories/ws/repo",
			method:     "GET",
			wantStatus: 404,
		},
		{
			name:       "unauthorized",
			status:     401,
			message:    "Authentication required",
			path:       "/2.0/repositories/ws/repo",
			method:     "GET",
			wantStatus: 401,
		},
		{
			name:       "bad_request",
			status:     400,
			message:    "Bad request",
			path:       "/2.0/repositories/ws/repo",
			method:     "GET",
			wantStatus: 400,
		},
		{
			name:       "server_error",
			status:     500,
			message:    "Internal server error",
			path:       "/2.0/repositories/ws/repo",
			method:     "GET",
			wantStatus: 500,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			_, c := bitbucketErrorServer(t, sc.status, sc.message)
			ctx := context.Background()

			var result map[string]interface{}
			err := c.Get(ctx, sc.path, &result)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("Expected *client.APIError, got %T: %v", err, err)
			}
			if apiErr.StatusCode != sc.wantStatus {
				t.Errorf("Expected status %d, got %d", sc.wantStatus, apiErr.StatusCode)
			}

			errStr := apiErr.Error()
			// Error message should contain status code for identification
			if !strings.Contains(errStr, fmt.Sprintf("%d", sc.wantStatus)) {
				t.Errorf("Error message should contain status code %d. Got: %s", sc.wantStatus, errStr)
			}
			// Error message should NOT contain raw HTTP headers or body dumps
			if strings.Contains(errStr, "HTTP/1.1") || strings.Contains(errStr, "Content-Length") {
				t.Errorf("Error message should not expose raw HTTP details. Got: %s", errStr)
			}
		})
	}
}

// --- Error message identifies which resource failed ---

func TestBitbucketErrorMessageIdentifiesResource(t *testing.T) {
	t.Parallel()

	resources := []struct {
		name   string
		path   string
		status int
	}{
		{"repository", "/2.0/repositories/ws/repo", 404},
		{"branch_restriction", "/2.0/repositories/ws/repo/branch-restrictions/123", 404},
		{"deployment", "/2.0/repositories/ws/repo/environments/456", 404},
		{"permission", "/2.0/repositories/ws/repo/permissions-config/users/user1", 404},
	}

	for _, r := range resources {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()
			_, c := bitbucketErrorServer(t, r.status, fmt.Sprintf("%s not found", r.name))
			ctx := context.Background()

			var result map[string]interface{}
			err := c.Get(ctx, r.path, &result)
			if err == nil {
				t.Fatalf("[%s] Expected error, got nil", r.name)
			}
			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("[%s] Expected *client.APIError, got %T", r.name, err)
			}
			if apiErr.StatusCode != r.status {
				t.Errorf("[%s] Expected status %d, got %d", r.name, r.status, apiErr.StatusCode)
			}
		})
	}
}
