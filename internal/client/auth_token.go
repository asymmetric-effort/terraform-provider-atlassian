package client

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

// TokenAuthenticator implements API token authentication for Atlassian Cloud.
// It uses the email:token pair encoded as base64 in the Authorization header.
type TokenAuthenticator struct {
	username string
	token    string
}

// NewTokenAuthenticator creates a new API token authenticator.
func NewTokenAuthenticator(username, token string) (*TokenAuthenticator, error) {
	if username == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'username' is required for API token authentication. " +
			"Set the 'username' attribute or the ATLASSIAN_USERNAME environment variable")
	}
	if token == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'api_token' is required for API token authentication. " +
			"Set the 'api_token' attribute or the ATLASSIAN_API_TOKEN environment variable")
	}
	return &TokenAuthenticator{username: username, token: token}, nil
}

// AuthenticateRequest adds the API token Authorization header to the request.
func (a *TokenAuthenticator) AuthenticateRequest(req *http.Request) error {
	credentials := base64.StdEncoding.EncodeToString([]byte(a.username + ":" + a.token))
	req.Header.Set("Authorization", "Basic "+credentials)
	return nil
}
