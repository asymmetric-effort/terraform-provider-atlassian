// Package unit contains targeted tests to close overall coverage gaps to >= 98%.
package unit

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock/specs"
)

// ============================================================================
// Client package: Post/Put/Delete with nil result (success path returns nil)
// ============================================================================

// TestClientPostNilResult exercises the Post path where result is nil.
func TestClientPostNilResultCoverage(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"1"}`))
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatal(err)
	}
	err = c.Post(context.Background(), "/test", strings.NewReader(`{}`), nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClientPostWithResult exercises the Post path where result is non-nil.
func TestClientPostWithResult(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "1"})
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	var result map[string]string
	err := c.Post(context.Background(), "/test", strings.NewReader(`{}`), &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result["id"] != "1" {
		t.Errorf("expected id '1', got %q", result["id"])
	}
}

// TestClientPostErrorResponse exercises Post with a non-2xx response.
func TestClientPostErrorResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": map[string]string{"name": "is required"},
		})
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	var result map[string]string
	err := c.Post(context.Background(), "/test", strings.NewReader(`{}`), &result)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected field-level error, got: %s", err.Error())
	}
}

// TestClientPutNilResult exercises the Put path where result is nil.
func TestClientPutNilResultCoverage(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"1"}`))
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	err := c.Put(context.Background(), "/test/1", strings.NewReader(`{}`), nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClientPutWithResult exercises the Put path where result is non-nil.
func TestClientPutWithResult(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": "1", "name": "updated"})
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	var result map[string]string
	err := c.Put(context.Background(), "/test/1", strings.NewReader(`{}`), &result)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result["name"] != "updated" {
		t.Errorf("expected name 'updated', got %q", result["name"])
	}
}

// TestClientPutErrorResponse exercises Put with a non-2xx response.
func TestClientPutErrorResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Not Found",
		})
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	var result map[string]string
	err := c.Put(context.Background(), "/test/1", strings.NewReader(`{}`), &result)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestClientDeleteSuccess exercises the Delete success path.
func TestClientDeleteSuccess(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	err := c.Delete(context.Background(), "/test/1")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// TestClientDeleteErrorResponse exercises Delete with a non-2xx response.
func TestClientDeleteErrorResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Forbidden",
		})
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	err := c.Delete(context.Background(), "/test/1")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "permission") {
		t.Errorf("expected permission error, got: %s", err.Error())
	}
}

// TestClientDoContextCancelDuringBackoff exercises context cancellation during retry backoff.
func TestClientDoContextCancelDuringBackoff(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 5
	cfg.RetryWaitMin = 5 * time.Second
	cfg.RetryWaitMax = 10 * time.Second
	c, _ := client.NewClient(cfg, &mockAuth{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.Do(ctx, http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context error, got: %s", err.Error())
	}
}

// TestClientDoRetryOn503 exercises the 503 retry path.
func TestClientDoRetryOn503Coverage(t *testing.T) {
	t.Parallel()
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 3
	cfg.RetryWaitMin = 10 * time.Millisecond
	cfg.RetryWaitMax = 20 * time.Millisecond
	c, _ := client.NewClient(cfg, &mockAuth{})

	resp, err := c.Do(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	resp.Body.Close()
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

// TestClientDoNetworkErrorRetry exercises the network-error retry path.
func TestClientDoNetworkErrorRetry(t *testing.T) {
	t.Parallel()
	// Use a server that immediately closes to cause network errors
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// close the connection abruptly by hijacking
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 1
	cfg.RetryWaitMin = 10 * time.Millisecond
	cfg.RetryWaitMax = 20 * time.Millisecond
	c, _ := client.NewClient(cfg, &mockAuth{})

	_, err := c.Do(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error after retries exhausted on network error")
	}
	if !strings.Contains(err.Error(), "failed after") {
		t.Errorf("expected retry exhaustion error, got: %s", err.Error())
	}
}

// ============================================================================
// OAuth: malformed JSON response from token endpoint
// ============================================================================

// TestOAuthRefreshMalformedJSON exercises the malformed JSON response path on refresh.
func TestOAuthRefreshMalformedJSON(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	})

	auth, _ := client.NewOAuthRefreshAuthenticator("cid", "csec", "rtok")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "unable to parse token response") {
		t.Errorf("expected parse error, got: %s", err.Error())
	}
}

// TestOAuthClientCredentialsMalformedJSON exercises the malformed JSON response path on client creds.
func TestOAuthClientCredentialsMalformedJSON(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{not valid json`))
	})

	auth, _ := client.NewOAuthClientCredentialsAuthenticator("cid", "csec")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "unable to parse token response") {
		t.Errorf("expected parse error, got: %s", err.Error())
	}
}

// TestOAuthRefreshNetworkError exercises unreachable token server for refresh flow.
func TestOAuthRefreshNetworkError(t *testing.T) {
	originalURL := client.OAuthTokenURL
	client.OAuthTokenURL = "http://127.0.0.1:1/oauth/token"
	t.Cleanup(func() { client.OAuthTokenURL = originalURL })

	auth, _ := client.NewOAuthRefreshAuthenticator("cid", "csec", "rtok")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "unable to reach") {
		t.Errorf("expected network error, got: %s", err.Error())
	}
}

// TestOAuthClientCredentialsNetworkError exercises unreachable token server for client creds.
func TestOAuthClientCredentialsNetworkError(t *testing.T) {
	originalURL := client.OAuthTokenURL
	client.OAuthTokenURL = "http://127.0.0.1:1/oauth/token"
	t.Cleanup(func() { client.OAuthTokenURL = originalURL })

	auth, _ := client.NewOAuthClientCredentialsAuthenticator("cid", "csec")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
	if !strings.Contains(err.Error(), "unable to reach") {
		t.Errorf("expected network error, got: %s", err.Error())
	}
}

// TestOAuthRefreshValidation tests NewOAuthRefreshAuthenticator constructor validation.
func TestOAuthRefreshValidation(t *testing.T) {
	t.Parallel()
	_, err := client.NewOAuthRefreshAuthenticator("", "csec", "rtok")
	if err == nil {
		t.Fatal("expected error for empty client_id")
	}
	_, err = client.NewOAuthRefreshAuthenticator("cid", "", "rtok")
	if err == nil {
		t.Fatal("expected error for empty client_secret")
	}
	_, err = client.NewOAuthRefreshAuthenticator("cid", "csec", "")
	if err == nil {
		t.Fatal("expected error for empty refresh_token")
	}
}

// TestOAuthClientCredentialsValidation tests NewOAuthClientCredentialsAuthenticator validation.
func TestOAuthClientCredentialsValidation(t *testing.T) {
	t.Parallel()
	_, err := client.NewOAuthClientCredentialsAuthenticator("", "csec")
	if err == nil {
		t.Fatal("expected error for empty client_id")
	}
	_, err = client.NewOAuthClientCredentialsAuthenticator("cid", "")
	if err == nil {
		t.Fatal("expected error for empty client_secret")
	}
}

// ============================================================================
// specs package: Valid() methods on generated enum types
// ============================================================================

// TestOAuthTokenRequestGrantTypeValid exercises the Valid() method on OAuthTokenRequestGrantType.
func TestOAuthTokenRequestGrantTypeValid(t *testing.T) {
	t.Parallel()
	if !specs.ClientCredentials.Valid() {
		t.Error("ClientCredentials should be valid")
	}
	if !specs.RefreshToken.Valid() {
		t.Error("RefreshToken should be valid")
	}
	invalid := specs.OAuthTokenRequestGrantType("bad")
	if invalid.Valid() {
		t.Error("bad grant type should be invalid")
	}
}

// TestCreateRoleAssignmentRequestPrincipalTypeValid exercises the Valid() method.
func TestCreateRoleAssignmentRequestPrincipalTypeValid(t *testing.T) {
	t.Parallel()
	if !specs.Group.Valid() {
		t.Error("Group should be valid")
	}
	if !specs.User.Valid() {
		t.Error("User should be valid")
	}
	invalid := specs.CreateRoleAssignmentRequestPrincipalType("bad")
	if invalid.Valid() {
		t.Error("bad principal type should be invalid")
	}
}

// TestCreateRoleAssignmentRequestScopeValid exercises the Valid() method.
func TestCreateRoleAssignmentRequestScopeValid(t *testing.T) {
	t.Parallel()
	if !specs.CreateRoleAssignmentRequestScopeOrg.Valid() {
		t.Error("org should be valid")
	}
	if !specs.CreateRoleAssignmentRequestScopeProduct.Valid() {
		t.Error("product should be valid")
	}
	invalid := specs.CreateRoleAssignmentRequestScope("bad")
	if invalid.Valid() {
		t.Error("bad scope should be invalid")
	}
}

// TestCreateRoleRequestScopeValid exercises the Valid() method.
func TestCreateRoleRequestScopeValid(t *testing.T) {
	t.Parallel()
	if !specs.CreateRoleRequestScopeOrg.Valid() {
		t.Error("org should be valid")
	}
	if !specs.CreateRoleRequestScopeProduct.Valid() {
		t.Error("product should be valid")
	}
	invalid := specs.CreateRoleRequestScope("bad")
	if invalid.Valid() {
		t.Error("bad scope should be invalid")
	}
}

// TestCreateBoardRequestTypeValid exercises the Valid() method.
func TestCreateBoardRequestTypeValid(t *testing.T) {
	t.Parallel()
	if !specs.Kanban.Valid() {
		t.Error("Kanban should be valid")
	}
	if !specs.Scrum.Valid() {
		t.Error("Scrum should be valid")
	}
	invalid := specs.CreateBoardRequestType("bad")
	if invalid.Valid() {
		t.Error("bad type should be invalid")
	}
}

// ============================================================================
// Mock Statuspage endpoints: error paths not covered by existing CRUD tests
// ============================================================================

func newFullStatuspageServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := mock.NewServer()
	mock.RegisterStatuspageEndpoints(s)
	return httptest.NewServer(s.Handler())
}

// TestSPComponentGroupCRUDFull exercises component group CRUD including error paths.
func TestSPComponentGroupCRUDFull(t *testing.T) {
	t.Parallel()
	ts := newFullStatuspageServer(t)
	defer ts.Close()

	pageID := "p1"

	// Create with missing body
	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups", "not json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for bad body, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create with missing component_group field
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups", map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing component_group, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create with missing name
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups", map[string]interface{}{
		"component_group": map[string]interface{}{"description": "d"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful create
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups", map[string]interface{}{
		"component_group": map[string]interface{}{"name": "CG1", "description": "desc"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var cg map[string]interface{}
	decodeJSON(t, resp, &cg)
	cgID := cg["id"].(string)

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/component-groups/" + cgID)
	if readResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Read not found
	readResp, _ = http.Get(ts.URL + "/v1/pages/" + pageID + "/component-groups/nonexistent")
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Update with bad body
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups/"+cgID, "not json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for bad update body, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update with missing component_group field
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups/"+cgID, map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing component_group, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update not found
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups/nonexistent", map[string]interface{}{
		"component_group": map[string]interface{}{"name": "Updated"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/component-groups/"+cgID, map[string]interface{}{
		"component_group": map[string]interface{}{"name": "Updated CG"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	listResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/component-groups")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// List with different page ID (empty)
	listResp, _ = http.Get(ts.URL + "/v1/pages/other/component-groups")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// Delete not found
	delResp := doDelete(t, ts.URL+"/v1/pages/"+pageID+"/component-groups/nonexistent")
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	// Successful delete
	delResp = doDelete(t, ts.URL+"/v1/pages/"+pageID+"/component-groups/"+cgID)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()
}

// TestSPSubscriberCRUDFull exercises subscriber CRUD including error paths.
func TestSPSubscriberCRUDFull(t *testing.T) {
	t.Parallel()
	ts := newFullStatuspageServer(t)
	defer ts.Close()
	pageID := "p1"

	// Create with bad JSON
	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers", "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create with missing subscriber field
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers", map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful create with component_ids
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers", map[string]interface{}{
		"subscriber": map[string]interface{}{
			"email":         "test@example.com",
			"endpoint":      "https://webhook.example.com",
			"component_ids": []string{"c1", "c2"},
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var sub map[string]interface{}
	decodeJSON(t, resp, &sub)
	subID := sub["id"].(string)

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/subscribers/" + subID)
	if readResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Read not found
	readResp, _ = http.Get(ts.URL + "/v1/pages/" + pageID + "/subscribers/nonexistent")
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Update bad body
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers/"+subID, "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update missing subscriber field
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers/"+subID, map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update not found
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers/nonexistent", map[string]interface{}{
		"subscriber": map[string]interface{}{"email": "new@example.com"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful update with component_ids
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/subscribers/"+subID, map[string]interface{}{
		"subscriber": map[string]interface{}{
			"email":         "updated@example.com",
			"component_ids": []string{"c3"},
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	listResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/subscribers")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// List with different page ID
	listResp, _ = http.Get(ts.URL + "/v1/pages/other/subscribers")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// Delete not found
	delResp := doDelete(t, ts.URL+"/v1/pages/"+pageID+"/subscribers/nonexistent")
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	// Successful delete
	delResp = doDelete(t, ts.URL+"/v1/pages/"+pageID+"/subscribers/"+subID)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()
}

// TestSPIncidentTemplateCRUDFull exercises incident template CRUD including error paths.
func TestSPIncidentTemplateCRUDFull(t *testing.T) {
	t.Parallel()
	ts := newFullStatuspageServer(t)
	defer ts.Close()
	pageID := "p1"

	// Create bad body
	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates", "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing template field
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates", map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing name
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates", map[string]interface{}{
		"template": map[string]interface{}{"title": "t"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful create
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates", map[string]interface{}{
		"template": map[string]interface{}{"name": "IT1", "title": "Title", "body": "Body"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var it map[string]interface{}
	decodeJSON(t, resp, &it)
	itID := it["id"].(string)

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/incident_templates/" + itID)
	if readResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Read not found
	readResp, _ = http.Get(ts.URL + "/v1/pages/" + pageID + "/incident_templates/nonexistent")
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Update bad body
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates/"+itID, "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update missing template field
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates/"+itID, map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update not found
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates/nonexistent", map[string]interface{}{
		"template": map[string]interface{}{"name": "Updated"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates/"+itID, map[string]interface{}{
		"template": map[string]interface{}{"name": "Updated IT", "title": "New Title"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	listResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/incident_templates")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// List with different page ID
	listResp, _ = http.Get(ts.URL + "/v1/pages/other/incident_templates")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// Delete not found
	delResp := doDelete(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates/nonexistent")
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	// Successful delete
	delResp = doDelete(t, ts.URL+"/v1/pages/"+pageID+"/incident_templates/"+itID)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()
}

// TestSPMaintenanceTemplateCRUDFull exercises maintenance template CRUD including error paths.
func TestSPMaintenanceTemplateCRUDFull(t *testing.T) {
	t.Parallel()
	ts := newFullStatuspageServer(t)
	defer ts.Close()
	pageID := "p1"

	// Create bad body
	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates", "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing template field
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates", map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing name
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates", map[string]interface{}{
		"template": map[string]interface{}{"title": "t"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful create
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates", map[string]interface{}{
		"template": map[string]interface{}{"name": "MT1", "title": "Title", "body": "Body"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var mt map[string]interface{}
	decodeJSON(t, resp, &mt)
	mtID := mt["id"].(string)

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/maintenance_templates/" + mtID)
	if readResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Read not found
	readResp, _ = http.Get(ts.URL + "/v1/pages/" + pageID + "/maintenance_templates/nonexistent")
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Update bad body
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/"+mtID, "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update missing template field
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/"+mtID, map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update not found
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/nonexistent", map[string]interface{}{
		"template": map[string]interface{}{"name": "Updated"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/"+mtID, map[string]interface{}{
		"template": map[string]interface{}{"name": "Updated MT"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	listResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/maintenance_templates")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// List with different page ID
	listResp, _ = http.Get(ts.URL + "/v1/pages/other/maintenance_templates")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// Delete not found
	delResp := doDelete(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/nonexistent")
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	// Successful delete
	delResp = doDelete(t, ts.URL+"/v1/pages/"+pageID+"/maintenance_templates/"+mtID)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()
}

// TestSPPermissionCRUDFull exercises statuspage permission CRUD including error paths.
func TestSPPermissionCRUDFull(t *testing.T) {
	t.Parallel()
	ts := newFullStatuspageServer(t)
	defer ts.Close()
	pageID := "p1"

	// Create bad body
	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions", "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing permission field
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions", map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing required fields
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions", map[string]interface{}{
		"permission": map[string]interface{}{"principal_type": "user"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful create
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions", map[string]interface{}{
		"permission": map[string]interface{}{
			"principal_type": "user",
			"principal_id":   "uid-1",
			"role":           "admin",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var perm map[string]interface{}
	decodeJSON(t, resp, &perm)
	permID := perm["id"].(string)

	// Read
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/permissions/" + permID)
	if readResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Read not found
	readResp, _ = http.Get(ts.URL + "/v1/pages/" + pageID + "/permissions/nonexistent")
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Update bad body
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions/"+permID, "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update missing permission field
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions/"+permID, map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update not found
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions/nonexistent", map[string]interface{}{
		"permission": map[string]interface{}{"role": "viewer"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Successful update
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/permissions/"+permID, map[string]interface{}{
		"permission": map[string]interface{}{"role": "viewer"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// List
	listResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/permissions")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// List with different page ID
	listResp, _ = http.Get(ts.URL + "/v1/pages/other/permissions")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// Delete not found
	delResp := doDelete(t, ts.URL+"/v1/pages/"+pageID+"/permissions/nonexistent")
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	// Successful delete
	delResp = doDelete(t, ts.URL+"/v1/pages/"+pageID+"/permissions/"+permID)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()
}

// TestSPPageErrorPaths exercises Statuspage page error paths not covered by main CRUD test.
func TestSPPageErrorPaths(t *testing.T) {
	t.Parallel()
	ts := newFullStatuspageServer(t)
	defer ts.Close()

	// Create with bad JSON body
	resp := postJSON(t, ts.URL+"/v1/pages", "not json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create with missing page field
	resp = postJSON(t, ts.URL+"/v1/pages", map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create with missing name
	resp = postJSON(t, ts.URL+"/v1/pages", map[string]interface{}{
		"page": map[string]interface{}{"subdomain": "test"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create with no subdomain (should auto-generate)
	resp = postJSON(t, ts.URL+"/v1/pages", map[string]interface{}{
		"page": map[string]interface{}{"name": "AutoSub"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var page map[string]interface{}
	decodeJSON(t, resp, &page)
	pageID := page["id"].(string)

	// Update bad JSON body
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID, "not json")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update missing page field
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID, map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update not found
	resp = putJSON(t, ts.URL+"/v1/pages/nonexistent", map[string]interface{}{
		"page": map[string]interface{}{"name": "N"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update subdomain (triggers url rewrite)
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID, map[string]interface{}{
		"page": map[string]interface{}{"subdomain": "newsub"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	var updated map[string]interface{}
	decodeJSON(t, resp, &updated)
	if updated["url"] != "https://newsub.statuspage.io" {
		t.Errorf("expected url with newsub, got %v", updated["url"])
	}

	// Delete not found
	delResp := doDelete(t, ts.URL+"/v1/pages/nonexistent")
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	// Read not found
	readResp, _ := http.Get(ts.URL + "/v1/pages/nonexistent")
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// List pages
	listResp, _ := http.Get(ts.URL + "/v1/pages")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()
}

// TestSPComponentErrorPaths exercises Statuspage component error paths.
func TestSPComponentErrorPaths(t *testing.T) {
	t.Parallel()
	ts := newFullStatuspageServer(t)
	defer ts.Close()
	pageID := "p1"

	// Create bad JSON
	resp := postJSON(t, ts.URL+"/v1/pages/"+pageID+"/components", "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing component field
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/components", map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create missing name
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/components", map[string]interface{}{
		"component": map[string]interface{}{"status": "operational"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create with all optional fields
	resp = postJSON(t, ts.URL+"/v1/pages/"+pageID+"/components", map[string]interface{}{
		"component": map[string]interface{}{
			"name":        "C1",
			"description": "desc",
			"status":      "major_outage",
			"group_id":    "g1",
		},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var comp map[string]interface{}
	decodeJSON(t, resp, &comp)
	compID := comp["id"].(string)

	// Update bad body
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/components/"+compID, "bad")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update missing component field
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/components/"+compID, map[string]interface{}{"foo": "bar"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Update not found
	resp = putJSON(t, ts.URL+"/v1/pages/"+pageID+"/components/nonexistent", map[string]interface{}{
		"component": map[string]interface{}{"name": "Updated"},
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Read not found
	readResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/components/nonexistent")
	if readResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", readResp.StatusCode)
	}
	readResp.Body.Close()

	// Delete not found
	delResp := doDelete(t, ts.URL+"/v1/pages/"+pageID+"/components/nonexistent")
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", delResp.StatusCode)
	}
	delResp.Body.Close()

	// Component list with page filter
	listResp, _ := http.Get(ts.URL + "/v1/pages/" + pageID + "/components")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()

	// Component list with different page (should be empty)
	listResp, _ = http.Get(ts.URL + "/v1/pages/other/components")
	if listResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", listResp.StatusCode)
	}
	listResp.Body.Close()
}

// ============================================================================
// Mock validator: edge cases
// ============================================================================

// TestValidatorOperationRequiredBodyMissing exercises the "required body missing" path.
func TestValidatorOperationRequiredBodyMissing(t *testing.T) {
	t.Parallel()
	yamlSpec := `
openapi: "3.0.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required:
                - name
              properties:
                name:
                  type: string
components:
  schemas: {}
`
	v, err := mock.NewRequestValidatorFromBytes([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Request with no body
	req, _ := http.NewRequest("POST", "/test", nil)
	req.ContentLength = 0
	err = v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for missing required body")
	}
	if !strings.Contains(err.Error(), "request body is required") {
		t.Errorf("expected required body error, got: %s", err.Error())
	}
}

// TestValidatorOperationInvalidJSON exercises the invalid JSON body path.
func TestValidatorOperationInvalidJSON(t *testing.T) {
	t.Parallel()
	yamlSpec := `
openapi: "3.0.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
components:
  schemas: {}
`
	v, err := mock.NewRequestValidatorFromBytes([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	req, _ := http.NewRequest("POST", "/test", io.NopCloser(strings.NewReader("not json")))
	req.ContentLength = 8
	err = v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid JSON body")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("expected JSON error, got: %s", err.Error())
	}
}

// TestValidatorOperationEnumValidation exercises enum validation paths.
func TestValidatorOperationEnumValidation(t *testing.T) {
	t.Parallel()
	yamlSpec := `
openapi: "3.0.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                status:
                  type: string
                  enum:
                    - active
                    - inactive
components:
  schemas: {}
`
	v, err := mock.NewRequestValidatorFromBytes([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Invalid enum value
	body := `{"status":"bad_value"}`
	req, _ := http.NewRequest("POST", "/test", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = int64(len(body))
	err = v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for invalid enum value")
	}
	if !strings.Contains(err.Error(), "not one of") {
		t.Errorf("expected enum error, got: %s", err.Error())
	}

	// Valid enum value
	body = `{"status":"active"}`
	req, _ = http.NewRequest("POST", "/test", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = int64(len(body))
	err = v.ValidateRequest(req)
	if err != nil {
		t.Errorf("expected no error for valid enum, got: %v", err)
	}

	// Non-string enum value
	body = `{"status":123}`
	req, _ = http.NewRequest("POST", "/test", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = int64(len(body))
	err = v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for non-string enum value")
	}
	if !strings.Contains(err.Error(), "expected string for enum") {
		t.Errorf("expected string type error, got: %s", err.Error())
	}
}

// TestValidatorNoMatchingContent exercises the path where no matching content type exists.
func TestValidatorNoMatchingContent(t *testing.T) {
	t.Parallel()
	yamlSpec := `
openapi: "3.0.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: false
        content:
          text/plain:
            schema:
              type: string
components:
  schemas: {}
`
	v, err := mock.NewRequestValidatorFromBytes([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	body := `{"name":"test"}`
	req, _ := http.NewRequest("POST", "/test", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = int64(len(body))
	err = v.ValidateRequest(req)
	if err != nil {
		t.Errorf("expected no error when no json content type matches, got: %v", err)
	}
}

// TestValidatorRefSchema exercises the $ref resolution path.
func TestValidatorRefSchema(t *testing.T) {
	t.Parallel()
	yamlSpec := `
openapi: "3.0.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/TestObj'
components:
  schemas:
    TestObj:
      type: object
      required:
        - name
      properties:
        name:
          type: string
`
	v, err := mock.NewRequestValidatorFromBytes([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	// Missing required field
	body := `{"other":"val"}`
	req, _ := http.NewRequest("POST", "/test", io.NopCloser(strings.NewReader(body)))
	req.ContentLength = int64(len(body))
	err = v.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if !strings.Contains(err.Error(), "missing required field") {
		t.Errorf("expected missing required field error, got: %s", err.Error())
	}
}

// TestValidatorNoBody exercises the path where request has no body and body is not required.
func TestValidatorNoBodyNotRequired(t *testing.T) {
	t.Parallel()
	yamlSpec := `
openapi: "3.0.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: false
        content:
          application/json:
            schema:
              type: object
components:
  schemas: {}
`
	v, err := mock.NewRequestValidatorFromBytes([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	req, _ := http.NewRequest("POST", "/test", nil)
	req.ContentLength = 0
	err = v.ValidateRequest(req)
	if err != nil {
		t.Errorf("expected no error for optional body, got: %v", err)
	}
}

// TestValidatorNoMatchingMethod exercises the path where method doesn't match.
func TestValidatorNoMatchingMethod(t *testing.T) {
	t.Parallel()
	yamlSpec := `
openapi: "3.0.0"
paths:
  /test:
    post:
      operationId: testPost
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
components:
  schemas: {}
`
	v, err := mock.NewRequestValidatorFromBytes([]byte(yamlSpec))
	if err != nil {
		t.Fatalf("failed to create validator: %v", err)
	}

	req, _ := http.NewRequest("GET", "/test", nil)
	err = v.ValidateRequest(req)
	if err != nil {
		t.Errorf("expected no error for unmatched method, got: %v", err)
	}
}

// ============================================================================
// Mock endpoint registration functions: exercises RegisterXxxEndpoints directly
// ============================================================================

// TestRegisterBitbucketEndpointsDirectly ensures RegisterBitbucketEndpoints doesn't panic.
func TestRegisterBitbucketEndpointsDirectly(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterBitbucketEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Hit the list endpoint to exercise registration
	resp, _ := http.Get(ts.URL + "/2.0/repositories/ws/slug")
	resp.Body.Close()
}

// TestRegisterConfluenceEndpointsDirectly ensures RegisterConfluenceEndpoints doesn't panic.
func TestRegisterConfluenceEndpointsDirectly(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterConfluenceEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/wiki/api/v2/spaces")
	resp.Body.Close()
}

// TestRegisterIdentityEndpointsDirectly ensures RegisterIdentityEndpoints doesn't panic.
func TestRegisterIdentityEndpointsDirectly(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/scim/directory/Users")
	resp.Body.Close()
}

// TestRegisterJiraEndpointsDirectly ensures RegisterJiraEndpoints doesn't panic.
func TestRegisterJiraEndpointsDirectly(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterJiraEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, _ := http.Get(ts.URL + "/rest/api/3/project")
	resp.Body.Close()
}

// ============================================================================
// Client: translateError with various error body shapes
// ============================================================================

// TestClientTranslateErrorWithFieldErrors exercises the errors map path.
func TestClientTranslateErrorWithFieldErrors(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": map[string]string{
				"field1": "is required",
				"field2": "is too long",
			},
		})
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	var result map[string]string
	err := c.Get(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestClientTranslateErrorNonJSON exercises the non-JSON body path.
func TestClientTranslateErrorNonJSON(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("plain text error"))
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &mockAuth{})

	var result map[string]string
	err := c.Get(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestAPIErrorFormatting exercises the APIError.Error() method with various combinations.
func TestAPIErrorFormatting(t *testing.T) {
	t.Parallel()

	// Error with resource and action
	err1 := &client.APIError{StatusCode: 404, Message: "not found", Resource: "/test", Action: "read"}
	if !strings.Contains(err1.Error(), "on /test") {
		t.Errorf("expected resource in error, got: %s", err1.Error())
	}
	if !strings.Contains(err1.Error(), "during read") {
		t.Errorf("expected action in error, got: %s", err1.Error())
	}

	// Error with no resource or action
	err2 := &client.APIError{StatusCode: 500, Message: "server error"}
	if strings.Contains(err2.Error(), "on ") {
		t.Errorf("unexpected 'on' in error: %s", err2.Error())
	}
	if strings.Contains(err2.Error(), "during ") {
		t.Errorf("unexpected 'during' in error: %s", err2.Error())
	}
}

// ============================================================================
// Client: auth failure in Do
// ============================================================================

type failAuth struct{}

func (f *failAuth) AuthenticateRequest(_ *http.Request) error {
	return io.ErrUnexpectedEOF
}

// TestClientDoAuthFailure exercises the auth failure path in Do.
func TestClientDoAuthFailure(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = ts.URL
	cfg.MaxRetries = 0
	c, _ := client.NewClient(cfg, &failAuth{})

	_, err := c.Do(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected auth failed, got: %s", err.Error())
	}
}

// ============================================================================
// Suppress unused import warnings
// ============================================================================

var _ = bytes.NewReader
var _ = io.NopCloser
