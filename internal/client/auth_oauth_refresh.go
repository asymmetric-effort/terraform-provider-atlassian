package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// OAuthTokenURL is the Atlassian OAuth token endpoint URL.
// It is a variable so tests can override it with a mock server URL.
var OAuthTokenURL = "https://auth.atlassian.com/oauth/token"

// OAuthRefreshAuthenticator implements OAuth 2.0 three-legged (3LO) refresh token auth.
// The user obtains a refresh token out-of-band; the authenticator handles access token
// retrieval and automatic refresh.
type OAuthRefreshAuthenticator struct {
	clientID     string
	clientSecret string
	refreshToken string

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewOAuthRefreshAuthenticator creates a new OAuth 2.0 refresh token authenticator.
func NewOAuthRefreshAuthenticator(clientID, clientSecret, refreshToken string) (*OAuthRefreshAuthenticator, error) {
	if clientID == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'oauth_client_id' is required for OAuth authentication. " +
			"Set the 'oauth_client_id' attribute or the ATLASSIAN_OAUTH_CLIENT_ID environment variable")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'oauth_client_secret' is required for OAuth authentication. " +
			"Set the 'oauth_client_secret' attribute or the ATLASSIAN_OAUTH_CLIENT_SECRET environment variable")
	}
	if refreshToken == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'oauth_refresh_token' is required for OAuth 3LO authentication. " +
			"Set the 'oauth_refresh_token' attribute or the ATLASSIAN_OAUTH_REFRESH_TOKEN environment variable")
	}
	return &OAuthRefreshAuthenticator{
		clientID:     clientID,
		clientSecret: clientSecret,
		refreshToken: refreshToken,
	}, nil
}

// AuthenticateRequest adds the OAuth Bearer token to the request, refreshing if needed.
func (a *OAuthRefreshAuthenticator) AuthenticateRequest(req *http.Request) error {
	token, err := a.getAccessToken()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// getAccessToken returns a valid access token, refreshing if expired.
func (a *OAuthRefreshAuthenticator) getAccessToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.accessToken != "" && time.Now().Before(a.expiresAt) {
		return a.accessToken, nil
	}

	return a.refreshAccessToken()
}

// refreshAccessToken exchanges the refresh token for a new access token.
func (a *OAuthRefreshAuthenticator) refreshAccessToken() (string, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     a.clientID,
		"client_secret": a.clientSecret,
		"refresh_token": a.refreshToken,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	resp, err := http.Post(OAuthTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("OAuth token refresh failed: unable to reach Atlassian auth server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)

		switch errResp.Error {
		case "invalid_grant":
			return "", fmt.Errorf("OAuth token refresh failed: refresh token is invalid or expired. " +
				"Obtain a new refresh token from the Atlassian developer console")
		case "invalid_client":
			return "", fmt.Errorf("OAuth token refresh failed: client credentials are invalid. " +
				"Check oauth_client_id and oauth_client_secret")
		default:
			return "", fmt.Errorf("OAuth token refresh failed (HTTP %d): %s — %s",
				resp.StatusCode, errResp.Error, errResp.Description)
		}
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("OAuth token refresh failed: unable to parse token response: %w", err)
	}

	a.accessToken = tokenResp.AccessToken
	// Refresh 30 seconds before actual expiry to avoid edge cases
	a.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-30) * time.Second)

	return a.accessToken, nil
}
