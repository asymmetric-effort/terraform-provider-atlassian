package mock

import (
	"encoding/json"
	"net/http"
	"time"
)

// ValidTestToken is the default valid API token for testing.
const ValidTestToken = "Basic dGVzdEBleGFtcGxlLmNvbTp0ZXN0LXRva2Vu" // test@example.com:test-token

// ValidBearerToken is the default valid OAuth bearer token for testing.
const ValidBearerToken = "Bearer mock-access-token"

// RegisterAuthEndpoints registers authentication endpoints on the mock server.
func RegisterAuthEndpoints(s *Server) {
	// API token validation: GET /rest/api/3/myself
	s.RegisterEndpoint("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			WriteError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		if auth != ValidTestToken && auth != ValidBearerToken {
			WriteError(w, http.StatusUnauthorized, "Invalid authentication credentials. "+
				"Check your API token or OAuth configuration")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"accountId":    "mock-account-id",
			"emailAddress": "test@example.com",
			"displayName":  "Test User",
			"active":       true,
		})
	})

	// OAuth token exchange: POST /oauth/token
	s.RegisterEndpoint("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			GrantType    string `json:"grant_type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RefreshToken string `json:"refresh_token"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]string{
				"error":             "invalid_request",
				"error_description": "Could not parse request body",
			})
			return
		}

		// Validate client credentials
		if req.ClientID != "mock-client-id" || req.ClientSecret != "mock-client-secret" {
			WriteJSON(w, http.StatusUnauthorized, map[string]string{
				"error":             "invalid_client",
				"error_description": "Invalid client credentials",
			})
			return
		}

		switch req.GrantType {
		case "refresh_token":
			if req.RefreshToken != "mock-refresh-token" {
				WriteJSON(w, http.StatusBadRequest, map[string]string{
					"error":             "invalid_grant",
					"error_description": "Refresh token is invalid or expired",
				})
				return
			}
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"access_token": "mock-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"scope":        "read:jira-work write:jira-work",
			})

		case "client_credentials":
			WriteJSON(w, http.StatusOK, map[string]interface{}{
				"access_token": "mock-access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})

		default:
			WriteJSON(w, http.StatusBadRequest, map[string]string{
				"error":             "unsupported_grant_type",
				"error_description": "Grant type '" + req.GrantType + "' is not supported",
			})
		}
	})

	// Rate limit simulation endpoint for testing
	var rateLimitCounter int
	s.RegisterEndpoint("GET /test/rate-limit", func(w http.ResponseWriter, r *http.Request) {
		rateLimitCounter++
		if rateLimitCounter <= 2 {
			w.Header().Set("Retry-After", "1")
			w.Header().Set("X-RateLimit-Reset", time.Now().Add(1*time.Second).Format(time.RFC3339))
			WriteError(w, http.StatusTooManyRequests, "Rate limit exceeded")
			return
		}
		rateLimitCounter = 0
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
}
