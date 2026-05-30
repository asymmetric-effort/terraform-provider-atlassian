package client

import (
	"encoding/base64"
	"fmt"
	"net/http"
)

// BasicAuthenticator implements HTTP Basic authentication for the
// Atlassian site API. It uses the username:api_key pair encoded as
// base64 in the Authorization header.
type BasicAuthenticator struct {
	username string
	apiKey   string
}

// NewBasicAuthenticator creates a new Basic authenticator for site API calls.
func NewBasicAuthenticator(username, apiKey string) (*BasicAuthenticator, error) {
	if username == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'username' is required for API key authentication. " +
			"Set the 'username' attribute or the ATLASSIAN_USERNAME environment variable")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'api_key' is required for API key authentication. " +
			"Set the 'api_key' attribute or the ATLASSIAN_API_KEY environment variable")
	}
	return &BasicAuthenticator{username: username, apiKey: apiKey}, nil
}

// AuthenticateRequest adds the Basic Authorization header to the request.
func (a *BasicAuthenticator) AuthenticateRequest(req *http.Request) error {
	credentials := base64.StdEncoding.EncodeToString([]byte(a.username + ":" + a.apiKey))
	req.Header.Set("Authorization", "Basic "+credentials)
	return nil
}
