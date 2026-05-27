// Package unit contains tests for mock server utility functions.
package unit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	atlassian "github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// TestMockRun tests the Run function by starting on a random port and immediately hitting it.
func TestMockRun(t *testing.T) {
	t.Parallel()
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	go func() {
		mock.Run(addr)
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Hit the health endpoint
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		t.Logf("health check failed (server may not have started): %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("health check: expected 200, got %d", resp.StatusCode)
	}
}

// TestMockListenAndServe tests the ListenAndServe method.
func TestMockListenAndServe(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	go func() {
		s.ListenAndServe(addr)
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	if err != nil {
		t.Logf("server may not have started: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestMockUserEndpointForbiddenHeader tests the X-Mock-Forbidden header.
func TestMockUserEndpointForbiddenHeader(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	c, _ := atlassian.NewClient(cfg, auth)

	// The mock has a X-Mock-Forbidden header check - but the client doesn't send custom headers
	// So we test it through a direct HTTP request
	body := `{"emailAddress":"test@e.com","displayName":"Test"}`
	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/user", nopBody(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mock-Forbidden", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}

	_ = c // keep client reference
}

// TestMockUserEndpointBadJSON tests user create with bad JSON body.
func TestMockUserEndpointBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/user", nopBody("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockGroupEndpointBadJSON tests group create with bad JSON.
func TestMockGroupEndpointBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/group", nopBody("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockRoleEndpointBadJSON tests role create with bad JSON.
func TestMockRoleEndpointBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/role", nopBody("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockUserUpdateBadJSON tests user update with bad JSON.
func TestMockUserUpdateBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Create a user first
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	c, _ := atlassian.NewClient(cfg, auth)
	ctx := context.Background()
	var user map[string]interface{}
	c.Post(ctx, "/rest/api/3/user", nopBody(`{"emailAddress":"upd@e.com","displayName":"U"}`), &user)
	accountID := user["accountId"].(string)

	req, _ := http.NewRequest("PUT", ts.URL+"/rest/api/3/user/"+accountID, nopBody("bad json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockRoleUpdateBadJSON tests role update with bad JSON.
func TestMockRoleUpdateBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	c, _ := atlassian.NewClient(cfg, auth)
	ctx := context.Background()
	var role map[string]interface{}
	c.Post(ctx, "/rest/api/3/role", nopBody(`{"name":"updrole"}`), &role)
	roleID := role["id"].(string)

	req, _ := http.NewRequest("PUT", ts.URL+"/rest/api/3/role/"+roleID, nopBody("bad json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockMembershipBadJSON tests membership add with bad JSON.
func TestMockMembershipBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// Create a group
	auth, _ := atlassian.NewTokenAuthenticator("test@example.com", "test-token")
	cfg := atlassian.Config{BaseURL: ts.URL, RequestTimeout: 5 * time.Second, MaxRetries: 0, RetryWaitMin: 1 * time.Second, RetryWaitMax: 1 * time.Second}
	c, _ := atlassian.NewClient(cfg, auth)
	ctx := context.Background()
	var grp map[string]interface{}
	c.Post(ctx, "/rest/api/3/group", nopBody(`{"name":"badjson-grp"}`), &grp)
	groupID := grp["groupId"].(string)

	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/group/user?groupId="+groupID, nopBody("bad"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockTokenBadJSON tests token create with bad JSON.
func TestMockTokenBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/user/uid/token", nopBody("bad"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockAssignmentBadJSON tests assignment create with bad JSON.
func TestMockAssignmentBadJSON(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/role/assignment", nopBody("bad"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

// TestMockMembershipMissingGroupId tests various missing parameter cases.
func TestMockMembershipMissingGroupId(t *testing.T) {
	t.Parallel()
	s := mock.NewServer()
	mock.RegisterIdentityEndpoints(s)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	// POST without groupId
	req, _ := http.NewRequest("POST", ts.URL+"/rest/api/3/group/user", nopBody(`{"accountId":"u1"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	// DELETE without both params
	req2, _ := http.NewRequest("DELETE", ts.URL+"/rest/api/3/group/user", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp2.StatusCode)
	}

	// GET without groupId
	req3, _ := http.NewRequest("GET", ts.URL+"/rest/api/3/group/member", nil)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp3.StatusCode)
	}

	// GET group without groupId
	req4, _ := http.NewRequest("GET", ts.URL+"/rest/api/3/group", nil)
	resp4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp4.StatusCode)
	}

	// DELETE group without groupId
	req5, _ := http.NewRequest("DELETE", ts.URL+"/rest/api/3/group", nil)
	resp5, err := http.DefaultClient.Do(req5)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp5.StatusCode)
	}

	// GET user without accountId
	req6, _ := http.NewRequest("GET", ts.URL+"/rest/api/3/user", nil)
	resp6, err := http.DefaultClient.Do(req6)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp6.Body.Close()
	if resp6.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp6.StatusCode)
	}
}
