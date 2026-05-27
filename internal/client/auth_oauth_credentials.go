package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// OAuthClientCredentialsAuthenticator implements OAuth 2.0 client credentials flow.
// This provides app-level auth without user context.
//
// Note: some Atlassian APIs require user-scoped tokens and may not work
// with client credentials auth.
type OAuthClientCredentialsAuthenticator struct {
	clientID     string
	clientSecret string

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

// NewOAuthClientCredentialsAuthenticator creates a new OAuth client credentials authenticator.
func NewOAuthClientCredentialsAuthenticator(clientID, clientSecret string) (*OAuthClientCredentialsAuthenticator, error) {
	if clientID == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'oauth_client_id' is required for OAuth authentication. " +
			"Set the 'oauth_client_id' attribute or the ATLASSIAN_OAUTH_CLIENT_ID environment variable")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'oauth_client_secret' is required for OAuth authentication. " +
			"Set the 'oauth_client_secret' attribute or the ATLASSIAN_OAUTH_CLIENT_SECRET environment variable")
	}
	return &OAuthClientCredentialsAuthenticator{
		clientID:     clientID,
		clientSecret: clientSecret,
	}, nil
}

// AuthenticateRequest adds the OAuth Bearer token to the request, refreshing if needed.
func (a *OAuthClientCredentialsAuthenticator) AuthenticateRequest(req *http.Request) error {
	token, err := a.getAccessToken()
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// getAccessToken returns a valid access token, fetching a new one if expired.
func (a *OAuthClientCredentialsAuthenticator) getAccessToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.accessToken != "" && time.Now().Before(a.expiresAt) {
		return a.accessToken, nil
	}

	return a.fetchAccessToken()
}

// fetchAccessToken obtains a new access token using client credentials.
func (a *OAuthClientCredentialsAuthenticator) fetchAccessToken() (string, error) {
	body, err := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     a.clientID,
		"client_secret": a.clientSecret,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}

	resp, err := http.Post(OAuthTokenURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("OAuth client credentials exchange failed: unable to reach Atlassian auth server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)

		switch errResp.Error {
		case "invalid_client":
			return "", fmt.Errorf("OAuth client credentials exchange failed: client credentials are invalid. " +
				"Check oauth_client_id and oauth_client_secret")
		case "unauthorized_client":
			return "", fmt.Errorf("OAuth client credentials exchange failed: this client is not authorized for client_credentials grant. " +
				"Verify the app is configured for client credentials flow in the Atlassian developer console")
		default:
			return "", fmt.Errorf("OAuth client credentials exchange failed (HTTP %d): %s — %s",
				resp.StatusCode, errResp.Error, errResp.Description)
		}
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("OAuth client credentials exchange failed: unable to parse token response: %w", err)
	}

	a.accessToken = tokenResp.AccessToken
	a.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn-30) * time.Second)

	return a.accessToken, nil
}
