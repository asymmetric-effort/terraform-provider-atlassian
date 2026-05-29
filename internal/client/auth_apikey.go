package client

import (
	"fmt"
	"net/http"
)

// APIKeyAuthenticator implements Bearer token authentication for the
// Atlassian Admin API. API keys are created at admin.atlassian.com
// under Organization settings > API Keys.
type APIKeyAuthenticator struct {
	apiKey string
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
func NewAPIKeyAuthenticator(apiKey string) (*APIKeyAuthenticator, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'api_key' is required for API key authentication. " +
			"Set the 'api_key' attribute or the ATLASSIAN_API_KEY environment variable")
	}
	return &APIKeyAuthenticator{apiKey: apiKey}, nil
}

// AuthenticateRequest adds the Bearer token Authorization header to the request.
func (a *APIKeyAuthenticator) AuthenticateRequest(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}
