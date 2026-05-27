// Package unit contains unit tests for identity resource error messages.
package unit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// newTestClient creates a client.Client pointed at the given mock server URL.
func newTestClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	cfg := client.DefaultConfig()
	cfg.BaseURL = baseURL
	cfg.MaxRetries = 0
	cfg.RequestTimeout = 5 * time.Second
	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	return c
}

// newErrorTestServer creates a mock server with identity endpoints for error testing.
func newErrorTestServer(t *testing.T) (*httptest.Server, *client.Client) {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	c := newTestClient(t, ts.URL)
	return ts, c
}

// newForbiddenServer creates a test server that returns 403 for all requests.
func newForbiddenServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.WriteJSON(w, http.StatusForbidden, map[string]interface{}{
			"errorMessages": []string{"Insufficient permissions"},
			"errors":        map[string]string{},
		})
	}))
}

// newNotFoundServer creates a test server that returns 404 for all requests.
func newNotFoundServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.WriteJSON(w, http.StatusNotFound, map[string]interface{}{
			"errorMessages": []string{"Resource not found"},
			"errors":        map[string]string{},
		})
	}))
}

// TestUserNotFoundError verifies the error returned when reading a non-existent user.
func TestUserNotFoundError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	var result map[string]interface{}
	err := c.Get(context.Background(), "/rest/api/3/user?accountId=nonexistent-user-999", &result)
	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status code 404, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "404") {
		t.Errorf("error message should contain status code '404': %s", msg)
	}
	if !strings.Contains(msg, "Verify") {
		t.Errorf("error message should suggest verification: %s", msg)
	}
}

// TestUserDuplicateEmailError verifies the error when creating a user with an existing email.
func TestUserDuplicateEmailError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	// Create a user first
	body := strings.NewReader(`{"emailAddress":"dupe@example.com","displayName":"First User"}`)
	var user map[string]interface{}
	err := c.Post(context.Background(), "/rest/api/3/user", body, &user)
	if err != nil {
		t.Fatalf("failed to create initial user: %v", err)
	}

	// Try to create another user with the same email
	body = strings.NewReader(`{"emailAddress":"dupe@example.com","displayName":"Second User"}`)
	var user2 map[string]interface{}
	err = c.Post(context.Background(), "/rest/api/3/user", body, &user2)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected status code 409, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "already exists") {
		t.Errorf("error message should indicate a duplicate exists: %s", msg)
	}
}

// TestUserPermissionDeniedError verifies the error when a 403 is returned.
func TestUserPermissionDeniedError(t *testing.T) {
	t.Parallel()
	ts := newForbiddenServer(t)
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	err := c.Delete(context.Background(), "/rest/api/3/user?accountId=some-user")
	if err == nil {
		t.Fatal("expected error for permission denied, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 403 {
		t.Errorf("expected status code 403, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "permission") {
		t.Errorf("error message should mention permission: %s", msg)
	}
	if !strings.Contains(msg, "403") {
		t.Errorf("error message should contain status code '403': %s", msg)
	}
}

// TestGroupNotFoundError verifies the error returned when reading a non-existent group.
func TestGroupNotFoundError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	var result map[string]interface{}
	err := c.Get(context.Background(), "/rest/api/3/group?groupId=nonexistent-group", &result)
	if err == nil {
		t.Fatal("expected error for non-existent group, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status code 404, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "not found") && !strings.Contains(msg, "Not Found") {
		t.Errorf("error message should indicate group was not found: %s", msg)
	}
}

// TestGroupDuplicateNameError verifies the error when creating a group with a duplicate name.
func TestGroupDuplicateNameError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	// Create a group first
	body := strings.NewReader(`{"name":"team-engineering"}`)
	var group map[string]interface{}
	err := c.Post(context.Background(), "/rest/api/3/group", body, &group)
	if err != nil {
		t.Fatalf("failed to create initial group: %v", err)
	}

	// Try to create another group with the same name
	body = strings.NewReader(`{"name":"team-engineering"}`)
	var group2 map[string]interface{}
	err = c.Post(context.Background(), "/rest/api/3/group", body, &group2)
	if err == nil {
		t.Fatal("expected error for duplicate group name, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected status code 409, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "already exists") {
		t.Errorf("error message should indicate a duplicate group exists: %s", msg)
	}
}

// TestRoleNotFoundError verifies the error returned when reading a non-existent role.
func TestRoleNotFoundError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	var result map[string]interface{}
	err := c.Get(context.Background(), "/rest/api/3/role/nonexistent-role", &result)
	if err == nil {
		t.Fatal("expected error for non-existent role, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status code 404, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "not found") && !strings.Contains(msg, "Not Found") {
		t.Errorf("error message should indicate role was not found: %s", msg)
	}
}

// TestRoleDuplicateNameError verifies the error when creating a role with a duplicate name.
func TestRoleDuplicateNameError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	// Create a role first
	body := strings.NewReader(`{"name":"admin-role","description":"Admin"}`)
	var role map[string]interface{}
	err := c.Post(context.Background(), "/rest/api/3/role", body, &role)
	if err != nil {
		t.Fatalf("failed to create initial role: %v", err)
	}

	// Try to create another role with the same name
	body = strings.NewReader(`{"name":"admin-role","description":"Another Admin"}`)
	var role2 map[string]interface{}
	err = c.Post(context.Background(), "/rest/api/3/role", body, &role2)
	if err == nil {
		t.Fatal("expected error for duplicate role name, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected status code 409, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "already exists") {
		t.Errorf("error message should indicate a duplicate role exists: %s", msg)
	}
}

// TestInvalidRoleAssignmentError verifies the error for a role assignment to a nonexistent endpoint.
func TestInvalidRoleAssignmentError(t *testing.T) {
	t.Parallel()
	ts := newNotFoundServer(t)
	defer ts.Close()

	c := newTestClient(t, ts.URL)
	body := strings.NewReader(`{"roleId":"nonexistent","principalType":"user","principalId":"user-1","scope":"org"}`)
	var result map[string]interface{}
	err := c.Post(context.Background(), "/rest/api/3/role/assignment", body, &result)
	if err == nil {
		t.Fatal("expected error for invalid role assignment, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status code 404, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "404") {
		t.Errorf("error message should contain status code: %s", msg)
	}
	if !strings.Contains(msg, "not found") && !strings.Contains(msg, "Not Found") {
		t.Errorf("error message should indicate resource not found: %s", msg)
	}
}

// TestTokenLimitExceededError verifies the error when a user exceeds the token limit.
func TestTokenLimitExceededError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	accountID := "token-limit-user"

	// Create 5 tokens to reach the limit
	for i := 0; i < 5; i++ {
		body := strings.NewReader(fmt.Sprintf(`{"label":"token-%d"}`, i))
		var token map[string]interface{}
		err := c.Post(context.Background(), fmt.Sprintf("/rest/api/3/user/%s/token", accountID), body, &token)
		if err != nil {
			t.Fatalf("failed to create token %d: %v", i, err)
		}
	}

	// Try to create a 6th token
	body := strings.NewReader(`{"label":"one-too-many"}`)
	var token map[string]interface{}
	err := c.Post(context.Background(), fmt.Sprintf("/rest/api/3/user/%s/token", accountID), body, &token)
	if err == nil {
		t.Fatal("expected error for token limit exceeded, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 409 {
		t.Errorf("expected status code 409, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "token limit") && !strings.Contains(lower, "maximum") {
		t.Errorf("error message should indicate token limit exceeded: %s", msg)
	}
}

// TestTokenNotFoundError verifies the error when reading a non-existent token.
func TestTokenNotFoundError(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	var result map[string]interface{}
	err := c.Get(context.Background(), "/rest/api/3/user/some-user/token/nonexistent-token", &result)
	if err == nil {
		t.Fatal("expected error for non-existent token, got nil")
	}

	apiErr, ok := err.(*client.APIError)
	if !ok {
		t.Fatalf("expected *client.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status code 404, got %d", apiErr.StatusCode)
	}

	msg := apiErr.Error()
	if !strings.Contains(msg, "not found") && !strings.Contains(msg, "Not Found") {
		t.Errorf("error message should indicate token was not found: %s", msg)
	}
}

// TestErrorMessageFormat verifies that all API errors follow a consistent format.
func TestErrorMessageFormat(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantParts  []string
	}{
		{
			name:       "user not found contains status code and action",
			path:       "/rest/api/3/user?accountId=missing",
			wantStatus: 404,
			wantParts:  []string{"HTTP 404", "not found", "Verify"},
		},
		{
			name:       "group not found contains status code and action",
			path:       "/rest/api/3/group?groupId=missing",
			wantStatus: 404,
			wantParts:  []string{"HTTP 404", "not found", "Verify"},
		},
		{
			name:       "role not found contains status code and action",
			path:       "/rest/api/3/role/missing",
			wantStatus: 404,
			wantParts:  []string{"HTTP 404", "not found", "Verify"},
		},
		{
			name:       "token not found contains status code and action",
			path:       "/rest/api/3/user/u1/token/missing",
			wantStatus: 404,
			wantParts:  []string{"HTTP 404", "not found", "Verify"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]interface{}
			err := c.Get(context.Background(), tt.path, &result)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("expected *client.APIError, got %T", err)
			}
			if apiErr.StatusCode != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, apiErr.StatusCode)
			}

			msg := apiErr.Error()
			for _, part := range tt.wantParts {
				if !strings.Contains(msg, part) {
					t.Errorf("error message missing expected part %q: %s", part, msg)
				}
			}

			// Verify no raw JSON/HTML is exposed
			if strings.Contains(msg, "<html") || strings.Contains(msg, "<HTML") {
				t.Errorf("error message should not expose raw HTML: %s", msg)
			}
			if strings.HasPrefix(msg, "{") || strings.HasPrefix(msg, "[") {
				t.Errorf("error message should not be raw JSON: %s", msg)
			}
		})
	}
}

// TestErrorMessagesNeverExposeRawJSON verifies that error messages from the mock server
// are translated by the client into user-friendly messages without raw API JSON.
func TestErrorMessagesNeverExposeRawJSON(t *testing.T) {
	t.Parallel()
	ts, c := newErrorTestServer(t)
	defer ts.Close()

	// Trigger various errors and verify format
	endpoints := []string{
		"/rest/api/3/user?accountId=no-such-user",
		"/rest/api/3/group?groupId=no-such-group",
		"/rest/api/3/role/no-such-role",
		"/rest/api/3/user/u1/token/no-such-token",
	}

	for _, ep := range endpoints {
		var result map[string]interface{}
		err := c.Get(context.Background(), ep, &result)
		if err == nil {
			t.Errorf("expected error for %s, got nil", ep)
			continue
		}

		msg := err.Error()

		// Must start with "atlassian API error" prefix from the client
		if !strings.HasPrefix(msg, "atlassian API error") {
			t.Errorf("error for %s should start with 'atlassian API error': %s", ep, msg)
		}

		// Must not contain raw JSON markers
		if strings.Contains(msg, `"errorMessages"`) {
			t.Errorf("error for %s should not contain raw JSON field 'errorMessages': %s", ep, msg)
		}
		if strings.Contains(msg, `"errors"`) {
			t.Errorf("error for %s should not contain raw JSON field 'errors': %s", ep, msg)
		}
	}
}

// TestPermissionDeniedErrorFormat verifies 403 errors have consistent format across resources.
func TestPermissionDeniedErrorFormat(t *testing.T) {
	t.Parallel()
	ts := newForbiddenServer(t)
	defer ts.Close()

	c := newTestClient(t, ts.URL)

	paths := []struct {
		name   string
		method string
		path   string
	}{
		{"user delete", "DELETE", "/rest/api/3/user?accountId=u1"},
		{"group delete", "DELETE", "/rest/api/3/group?groupId=g1"},
		{"role delete", "DELETE", "/rest/api/3/role/r1"},
		{"token delete", "DELETE", "/rest/api/3/user/u1/token/t1"},
	}

	for _, tt := range paths {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			switch tt.method {
			case "DELETE":
				err = c.Delete(context.Background(), tt.path)
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			apiErr, ok := err.(*client.APIError)
			if !ok {
				t.Fatalf("expected *client.APIError, got %T", err)
			}
			if apiErr.StatusCode != 403 {
				t.Errorf("expected status 403, got %d", apiErr.StatusCode)
			}

			msg := apiErr.Error()
			if !strings.Contains(msg, "403") {
				t.Errorf("error should contain status code 403: %s", msg)
			}
			if !strings.Contains(msg, "permission") {
				t.Errorf("error should mention permission: %s", msg)
			}
		})
	}
}
