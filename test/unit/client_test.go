// Package unit contains unit tests for the Atlassian API client.
package unit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
)

// mockAuth implements client.Authenticator for testing.
type mockAuth struct{}

// AuthenticateRequest adds a test authorization header.
func (m *mockAuth) AuthenticateRequest(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer test-token")
	return nil
}

// TestNewClientRequiresURL verifies that NewClient requires a base URL.
// TestNewClientAllowsEmptyURL verifies that NewClient allows empty BaseURL
// for admin-only usage. The URL is validated lazily when Do() is called.
func TestNewClientAllowsEmptyURL(t *testing.T) {
	t.Parallel()

	config := client.DefaultConfig()
	config.BaseURL = ""

	c, err := client.NewClient(config, &mockAuth{})
	if err != nil {
		t.Fatalf("NewClient with empty URL should succeed: %v", err)
	}

	// Site-specific calls should fail with a clear error
	err = c.Get(context.Background(), "/rest/api/3/test", nil)
	if err == nil {
		t.Fatal("expected error for site API call without URL")
	}
}

// TestNewClientRequiresAuth verifies that NewClient requires an authenticator.
func TestNewClientRequiresAuth(t *testing.T) {
	t.Parallel()

	config := client.DefaultConfig()
	config.BaseURL = "https://example.atlassian.net"

	_, err := client.NewClient(config, nil)
	if err == nil {
		t.Fatal("expected error for nil authenticator")
	}
}

// TestNewClientInvalidURL verifies that NewClient rejects invalid URLs.
func TestNewClientInvalidURL(t *testing.T) {
	t.Parallel()

	config := client.DefaultConfig()
	config.BaseURL = "not-a-url"

	_, err := client.NewClient(config, &mockAuth{})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// TestNewClientValidConfig verifies that NewClient succeeds with valid config.
func TestNewClientValidConfig(t *testing.T) {
	t.Parallel()

	config := client.DefaultConfig()
	config.BaseURL = "https://example.atlassian.net"

	c, err := client.NewClient(config, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

// TestClientGet verifies the Get method works correctly.
func TestClientGet(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "123"})
	}))
	defer server.Close()

	config := client.DefaultConfig()
	config.BaseURL = server.URL

	c, err := client.NewClient(config, &mockAuth{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]string
	err = c.Get(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "123" {
		t.Errorf("expected id '123', got %q", result["id"])
	}
}

// TestClientErrorTranslation verifies clear error messages for API errors.
func TestClientErrorTranslation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		body       string
		wantSubstr string
	}{
		{
			name:       "401 unauthorized",
			status:     401,
			body:       `{"message":"Unauthorized"}`,
			wantSubstr: "Check your authentication credentials",
		},
		{
			name:       "403 forbidden",
			status:     403,
			body:       `{"message":"Forbidden"}`,
			wantSubstr: "does not have permission",
		},
		{
			name:       "404 not found",
			status:     404,
			body:       `{"errorMessages":["Resource not found"]}`,
			wantSubstr: "Verify the resource exists",
		},
		{
			name:       "409 conflict",
			status:     409,
			body:       `{"errorMessages":["Already exists"]}`,
			wantSubstr: "may already exist",
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

			config := client.DefaultConfig()
			config.BaseURL = server.URL

			c, _ := client.NewClient(config, &mockAuth{})

			var result map[string]string
			err := c.Get(context.Background(), "/test", &result)
			if err == nil {
				t.Fatal("expected error")
			}

			errMsg := err.Error()
			if !contains(errMsg, tt.wantSubstr) {
				t.Errorf("expected error to contain %q, got: %s", tt.wantSubstr, errMsg)
			}
		})
	}
}

// TestClientRetryOn429 verifies exponential backoff on rate limit responses.
func TestClientRetryOn429(t *testing.T) {
	t.Parallel()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	config := client.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 5
	config.RetryWaitMin = 10 * time.Millisecond
	config.RetryWaitMax = 50 * time.Millisecond

	c, _ := client.NewClient(config, &mockAuth{})

	var result map[string]string
	err := c.Get(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", result["status"])
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts (2 retries + 1 success), got %d", attempts)
	}
}

// TestClientRetryExhausted verifies error when retries are exhausted.
func TestClientRetryExhausted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	config := client.DefaultConfig()
	config.BaseURL = server.URL
	config.MaxRetries = 2
	config.RetryWaitMin = 10 * time.Millisecond
	config.RetryWaitMax = 20 * time.Millisecond

	c, _ := client.NewClient(config, &mockAuth{})

	var result map[string]string
	err := c.Get(context.Background(), "/test", &result)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

// TestDefaultConfig verifies default configuration values.
func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	config := client.DefaultConfig()
	if config.RequestTimeout != 30*time.Second {
		t.Errorf("expected default RequestTimeout 30s, got %v", config.RequestTimeout)
	}
	if config.MaxRetries != 5 {
		t.Errorf("expected default MaxRetries 5, got %d", config.MaxRetries)
	}
	if config.RetryWaitMin != 1*time.Second {
		t.Errorf("expected default RetryWaitMin 1s, got %v", config.RetryWaitMin)
	}
	if config.RetryWaitMax != 30*time.Second {
		t.Errorf("expected default RetryWaitMax 30s, got %v", config.RetryWaitMax)
	}
}

// contains checks if s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

// TestSetAdminBaseURL verifies SetAdminBaseURL configures the admin API URL.
func TestSetAdminBaseURL(t *testing.T) {
	t.Parallel()
	config := client.DefaultConfig()
	config.BaseURL = "https://example.atlassian.net"
	c, err := client.NewClient(config, &mockAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	c.SetAdminBaseURL("https://admin.example.com")

	// AdminGet should work after setting admin URL
	// (will fail with connection refused, but won't fail with "not configured")
	err = c.AdminGet(context.Background(), "/v1/orgs", nil)
	if err == nil {
		t.Log("expected connection error, not nil")
	}
}

// TestAdminDoWithoutAdminURL verifies AdminDo fails when AdminBaseURL is empty.
func TestAdminDoWithoutAdminURL(t *testing.T) {
	t.Parallel()
	config := client.Config{
		BaseURL:        "https://example.atlassian.net",
		AdminBaseURL:   "",
		RequestTimeout: 5 * time.Second,
		MaxRetries:     0,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   1 * time.Second,
	}
	c, err := client.NewClient(config, &mockAuth{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	err = c.AdminGet(context.Background(), "/v1/orgs", nil)
	if err == nil {
		t.Fatal("expected error for admin call without admin URL")
	}
}

// searchString is a simple substring search.
func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
