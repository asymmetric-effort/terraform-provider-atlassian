// Package unit contains unit tests for OAuth authenticator implementations.
package unit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
)

// setupOAuthMockServer creates a mock OAuth token server and overrides the global URL.
// Returns the server and a cleanup function.
func setupOAuthMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	originalURL := client.OAuthTokenURL
	client.OAuthTokenURL = ts.URL + "/oauth/token"
	t.Cleanup(func() { client.OAuthTokenURL = originalURL })
	return ts
}

// TestOAuthRefreshSuccess tests successful OAuth refresh token flow.
func TestOAuthRefreshSuccess(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			GrantType    string `json:"grant_type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RefreshToken string `json:"refresh_token"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.GrantType != "refresh_token" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "unsupported_grant_type"})
			return
		}
		if req.ClientID != "cid" || req.ClientSecret != "csec" {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_client"})
			return
		}
		if req.RefreshToken != "rtok" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "mock-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})

	auth, err := client.NewOAuthRefreshAuthenticator("cid", "csec", "rtok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err = auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer mock-access-token" {
		t.Errorf("expected 'Bearer mock-access-token', got %q", got)
	}

	// Second call should use cached token
	req2, _ := http.NewRequest("GET", "https://example.com", nil)
	err = auth.AuthenticateRequest(req2)
	if err != nil {
		t.Fatalf("second AuthenticateRequest: %v", err)
	}
	if got := req2.Header.Get("Authorization"); got != "Bearer mock-access-token" {
		t.Errorf("expected cached token, got %q", got)
	}
}

// TestOAuthRefreshInvalidGrant tests the invalid_grant error path.
func TestOAuthRefreshInvalidGrant(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_grant",
			"error_description": "token expired",
		})
	})

	auth, _ := client.NewOAuthRefreshAuthenticator("cid", "csec", "bad-token")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "invalid or expired") {
		t.Errorf("expected invalid_grant message, got: %s", err.Error())
	}
}

// TestOAuthRefreshInvalidClient tests the invalid_client error path.
func TestOAuthRefreshInvalidClient(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_client",
			"error_description": "bad creds",
		})
	})

	auth, _ := client.NewOAuthRefreshAuthenticator("bad-cid", "bad-csec", "rtok")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "client credentials are invalid") {
		t.Errorf("expected invalid_client message, got: %s", err.Error())
	}
}

// TestOAuthRefreshUnknownError tests generic error path.
func TestOAuthRefreshUnknownError(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "server_error",
			"error_description": "unknown",
		})
	})

	auth, _ := client.NewOAuthRefreshAuthenticator("cid", "csec", "rtok")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "server_error") {
		t.Errorf("expected server_error in message, got: %s", err.Error())
	}
}

// TestOAuthClientCredentialsSuccess tests successful client credentials flow.
func TestOAuthClientCredentialsSuccess(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			GrantType    string `json:"grant_type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		if req.GrantType != "client_credentials" {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "cc-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	})

	auth, err := client.NewOAuthClientCredentialsAuthenticator("cid", "csec")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err = auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer cc-access-token" {
		t.Errorf("expected 'Bearer cc-access-token', got %q", got)
	}

	// Second call should use cached token
	req2, _ := http.NewRequest("GET", "https://example.com", nil)
	err = auth.AuthenticateRequest(req2)
	if err != nil {
		t.Fatalf("second AuthenticateRequest: %v", err)
	}
}

// TestOAuthClientCredentialsInvalidClient tests the invalid_client error path.
func TestOAuthClientCredentialsInvalidClient(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_client",
			"error_description": "bad creds",
		})
	})

	auth, _ := client.NewOAuthClientCredentialsAuthenticator("bad", "bad")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "client credentials are invalid") {
		t.Errorf("expected invalid_client message, got: %s", err.Error())
	}
}

// TestOAuthClientCredentialsUnauthorizedClient tests the unauthorized_client error path.
func TestOAuthClientCredentialsUnauthorizedClient(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "unauthorized_client",
			"error_description": "not allowed",
		})
	})

	auth, _ := client.NewOAuthClientCredentialsAuthenticator("cid", "csec")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "not authorized for client_credentials") {
		t.Errorf("expected unauthorized_client message, got: %s", err.Error())
	}
}

// TestOAuthClientCredentialsUnknownError tests generic error path.
func TestOAuthClientCredentialsUnknownError(t *testing.T) {
	setupOAuthMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "server_error",
			"error_description": "unknown",
		})
	})

	auth, _ := client.NewOAuthClientCredentialsAuthenticator("cid", "csec")
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err := auth.AuthenticateRequest(req)
	if err == nil {
		t.Fatal("expected error")
	}
	if !contains(err.Error(), "server_error") {
		t.Errorf("expected server_error, got: %s", err.Error())
	}
}

// TestAPIKeyAuthenticatorSetsBearerHeader verifies the Authorization header format.
func TestAPIKeyAuthenticatorSetsBearerHeader(t *testing.T) {
	t.Parallel()
	auth, err := client.NewAPIKeyAuthenticator("test-api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	err = auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	authHeader := req.Header.Get("Authorization")
	if authHeader != "Bearer test-api-key" {
		t.Errorf("expected 'Bearer test-api-key', got %q", authHeader)
	}
}
