// Package unit contains unit tests for the client GetPaginated method.
package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
)

// TestClientGetPaginated tests the GetPaginated method with multiple pages.
func TestClientGetPaginated(t *testing.T) {
	t.Parallel()

	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/items":
			page++
			response := map[string]interface{}{
				"values":  []map[string]string{{"id": "item-1"}, {"id": "item-2"}},
				"nextUrl": "/api/items/page2",
			}
			json.NewEncoder(w).Encode(response)
		case "/api/items/page2":
			page++
			response := map[string]interface{}{
				"values":  []map[string]string{{"id": "item-3"}},
				"nextUrl": "",
			}
			json.NewEncoder(w).Encode(response)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0
	cfg.RetryWaitMin = 10 * time.Millisecond
	cfg.RetryWaitMax = 50 * time.Millisecond

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	extractItems := func(raw json.RawMessage) ([]json.RawMessage, string, error) {
		var resp struct {
			Values  []json.RawMessage `json:"values"`
			NextURL string            `json:"nextUrl"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, "", err
		}
		return resp.Values, resp.NextURL, nil
	}

	items, err := c.GetPaginated(context.Background(), "/api/items", extractItems)
	if err != nil {
		t.Fatalf("GetPaginated failed: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	if page != 2 {
		t.Errorf("expected 2 page requests, got %d", page)
	}
}

// TestClientGetPaginatedSinglePage tests GetPaginated with a single page.
func TestClientGetPaginatedSinglePage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"values":  []map[string]string{{"id": "item-1"}},
			"nextUrl": "",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	extractItems := func(raw json.RawMessage) ([]json.RawMessage, string, error) {
		var resp struct {
			Values  []json.RawMessage `json:"values"`
			NextURL string            `json:"nextUrl"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, "", err
		}
		return resp.Values, resp.NextURL, nil
	}

	items, err := c.GetPaginated(context.Background(), "/api/items", extractItems)
	if err != nil {
		t.Fatalf("GetPaginated failed: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

// TestClientGetPaginatedError tests GetPaginated when the API returns an error.
func TestClientGetPaginatedError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"message": "Internal Server Error"})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	extractItems := func(raw json.RawMessage) ([]json.RawMessage, string, error) {
		return nil, "", nil
	}

	_, err = c.GetPaginated(context.Background(), "/api/items", extractItems)
	if err == nil {
		t.Fatal("expected error from GetPaginated")
	}
}

// TestClientGetPaginatedExtractError tests GetPaginated when the extract function returns an error.
func TestClientGetPaginatedExtractError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"values": []string{"item-1"}})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	extractItems := func(raw json.RawMessage) ([]json.RawMessage, string, error) {
		return nil, "", fmt.Errorf("extract error")
	}

	_, err = c.GetPaginated(context.Background(), "/api/items", extractItems)
	if err == nil {
		t.Fatal("expected error from GetPaginated")
	}
	if !contains(err.Error(), "extract error") {
		t.Errorf("expected error to contain 'extract error', got: %s", err.Error())
	}
}

// TestClientPostWithBody tests the Post method with a body and result.
func TestClientPostWithBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "new-123"})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	err = c.Post(context.Background(), "/api/create", nil, &result)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
	if result["id"] != "new-123" {
		t.Errorf("expected id 'new-123', got %q", result["id"])
	}
}

// TestClientPostNilResult tests the Post method with nil result (fire-and-forget).
func TestClientPostNilResult(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = c.Post(context.Background(), "/api/create", nil, nil)
	if err != nil {
		t.Fatalf("Post with nil result failed: %v", err)
	}
}

// TestClientPutWithBody tests the Put method.
func TestClientPutWithBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "updated-123"})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	err = c.Put(context.Background(), "/api/update", nil, &result)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if result["id"] != "updated-123" {
		t.Errorf("expected id 'updated-123', got %q", result["id"])
	}
}

// TestClientPutNilResult tests the Put method with nil result.
func TestClientPutNilResult(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = c.Put(context.Background(), "/api/update", nil, nil)
	if err != nil {
		t.Fatalf("Put with nil result failed: %v", err)
	}
}

// TestClientDelete tests the Delete method.
func TestClientDelete(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = c.Delete(context.Background(), "/api/delete")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

// TestClientDeleteError tests the Delete method with error response.
func TestClientDeleteError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Not Found"},
		})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = c.Delete(context.Background(), "/api/delete")
	if err == nil {
		t.Fatal("expected error from Delete")
	}
}

// TestClientPostError tests the Post method with error response.
func TestClientPostError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": map[string]string{"field": "invalid value"},
		})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = c.Post(context.Background(), "/api/create", nil, nil)
	if err == nil {
		t.Fatal("expected error from Post")
	}
}

// TestClientPutError tests the Put method with error response.
func TestClientPutError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Forbidden",
		})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 0

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = c.Put(context.Background(), "/api/update", nil, nil)
	if err == nil {
		t.Fatal("expected error from Put")
	}
}

// TestClientRetryOn503 tests retry on 503 responses.
func TestClientRetryOn503(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 3
	cfg.RetryWaitMin = 10 * time.Millisecond
	cfg.RetryWaitMax = 50 * time.Millisecond

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	err = c.Get(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

// TestClientContextCancellation tests that requests respect context cancellation.
func TestClientContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	cfg := client.DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.MaxRetries = 10
	cfg.RetryWaitMin = 1 * time.Second
	cfg.RetryWaitMax = 5 * time.Second

	c, err := client.NewClient(cfg, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var result map[string]string
	err = c.Get(ctx, "/test", &result)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestAPIErrorFormat tests the APIError.Error() method.
func TestAPIErrorFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     client.APIError
		wantSub string
	}{
		{
			name:    "basic error",
			err:     client.APIError{StatusCode: 500, Message: "Internal Server Error"},
			wantSub: "HTTP 500",
		},
		{
			name:    "error with resource",
			err:     client.APIError{StatusCode: 404, Message: "Not Found", Resource: "/api/user"},
			wantSub: "on /api/user",
		},
		{
			name:    "error with action",
			err:     client.APIError{StatusCode: 403, Message: "Forbidden", Action: "delete"},
			wantSub: "during delete",
		},
		{
			name:    "error with resource and action",
			err:     client.APIError{StatusCode: 409, Message: "Conflict", Resource: "/api/group", Action: "create"},
			wantSub: "on /api/group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := tt.err.Error()
			if !contains(msg, tt.wantSub) {
				t.Errorf("expected error to contain %q, got: %s", tt.wantSub, msg)
			}
		})
	}
}

// TestTranslateErrorFormats tests various error response formats from the mock.
func TestTranslateErrorFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		wantSub string
	}{
		{
			name:    "message field",
			status:  400,
			body:    `{"message":"bad request"}`,
			wantSub: "bad request",
		},
		{
			name:    "errorMessages field",
			status:  400,
			body:    `{"errorMessages":["first error","second error"]}`,
			wantSub: "first error",
		},
		{
			name:    "errors map field",
			status:  400,
			body:    `{"errors":{"name":"is required"}}`,
			wantSub: "is required",
		},
		{
			name:    "non-json body",
			status:  500,
			body:    `not json`,
			wantSub: "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			cfg := client.DefaultConfig()
			cfg.BaseURL = server.URL
			cfg.MaxRetries = 0

			c, _ := client.NewClient(cfg, &mockAuth{})

			var result map[string]string
			err := c.Get(context.Background(), "/test", &result)
			if err == nil {
				t.Fatal("expected error")
			}
			if !contains(err.Error(), tt.wantSub) {
				t.Errorf("expected error to contain %q, got: %s", tt.wantSub, err.Error())
			}
		})
	}
}
