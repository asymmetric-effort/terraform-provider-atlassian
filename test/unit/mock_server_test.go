// Package unit contains unit tests for the mock API server.
package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/mock"
)

// TestMockServerHealthCheck verifies the health check endpoint.
func TestMockServerHealthCheck(t *testing.T) {
	t.Parallel()

	s := mock.NewServer()
	server := httptest.NewServer(s.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", result["status"])
	}
}

// TestMockStoreBasicCRUD verifies in-memory store operations.
func TestMockStoreBasicCRUD(t *testing.T) {
	t.Parallel()

	store := mock.NewStore()

	// Set
	store.Set("1", json.RawMessage(`{"name":"test"}`))

	// Get
	item, ok := store.Get("1")
	if !ok {
		t.Fatal("expected item to exist")
	}
	var data map[string]string
	json.Unmarshal(item, &data)
	if data["name"] != "test" {
		t.Errorf("expected name 'test', got %q", data["name"])
	}

	// List
	items := store.List()
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}

	// Delete
	ok = store.Delete("1")
	if !ok {
		t.Fatal("expected delete to return true")
	}

	_, ok = store.Get("1")
	if ok {
		t.Fatal("expected item to be deleted")
	}

	// Delete non-existent
	ok = store.Delete("999")
	if ok {
		t.Fatal("expected delete of non-existent to return false")
	}
}

// TestMockServerEndpointRegistration verifies pluggable endpoint registration.
func TestMockServerEndpointRegistration(t *testing.T) {
	t.Parallel()

	s := mock.NewServer()
	s.RegisterEndpoint("GET /test/custom", func(w http.ResponseWriter, r *http.Request) {
		mock.WriteJSON(w, http.StatusOK, map[string]string{"custom": "endpoint"})
	})

	server := httptest.NewServer(s.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/test/custom")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["custom"] != "endpoint" {
		t.Errorf("expected 'endpoint', got %q", result["custom"])
	}
}

// TestMockServerGetStore verifies store creation and reuse.
func TestMockServerGetStore(t *testing.T) {
	t.Parallel()

	s := mock.NewServer()

	store1 := s.GetStore("users")
	store2 := s.GetStore("users")

	store1.Set("key", json.RawMessage(`"value"`))
	_, ok := store2.Get("key")
	if !ok {
		t.Fatal("expected same store instance to be returned")
	}
}

// TestMockAuthEndpointValidToken verifies auth with valid API token.
func TestMockAuthEndpointValidToken(t *testing.T) {
	t.Parallel()

	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	server := httptest.NewServer(s.Handler())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/rest/api/3/myself", nil)
	req.Header.Set("Authorization", mock.ValidTestAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

// TestMockAuthEndpointInvalidToken verifies auth rejection with invalid token.
func TestMockAuthEndpointInvalidToken(t *testing.T) {
	t.Parallel()

	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	server := httptest.NewServer(s.Handler())
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/rest/api/3/myself", nil)
	req.Header.Set("Authorization", "Basic invalid")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

// TestMockAuthEndpointNoAuth verifies auth rejection with no credentials.
func TestMockAuthEndpointNoAuth(t *testing.T) {
	t.Parallel()

	s := mock.NewServer()
	mock.RegisterAuthEndpoints(s)
	server := httptest.NewServer(s.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/rest/api/3/myself")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}
