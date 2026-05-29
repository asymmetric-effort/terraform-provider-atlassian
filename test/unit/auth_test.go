// Package unit contains unit tests for authentication implementations.
package unit

import (
	"net/http"
	"testing"

	"github.com/asymmetric-effort/terraform-provider-atlassian/internal/client"
)

// TestAPIKeyAuthenticatorValid verifies API key auth works with valid key.
func TestAPIKeyAuthenticatorValid(t *testing.T) {
	t.Parallel()

	auth, err := client.NewAPIKeyAuthenticator("test-api-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	err = auth.AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		t.Fatal("expected Authorization header to be set")
	}
	if authHeader != "Bearer test-api-key" {
		t.Errorf("expected 'Bearer test-api-key', got %q", authHeader)
	}
}

// TestAPIKeyAuthenticatorMissingKey verifies clear error for missing API key.
func TestAPIKeyAuthenticatorMissingKey(t *testing.T) {
	t.Parallel()

	_, err := client.NewAPIKeyAuthenticator("")
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
	if !contains(err.Error(), "api_key") {
		t.Errorf("expected error to mention 'api_key', got: %s", err.Error())
	}
}

// TestOAuthRefreshMissingClientID verifies clear error for missing client ID.
func TestOAuthRefreshMissingClientID(t *testing.T) {
	t.Parallel()

	_, err := client.NewOAuthRefreshAuthenticator("", "secret", "refresh")
	if err == nil {
		t.Fatal("expected error for missing client ID")
	}
	if !contains(err.Error(), "oauth_client_id") {
		t.Errorf("expected error to mention 'oauth_client_id', got: %s", err.Error())
	}
}

// TestOAuthRefreshMissingSecret verifies clear error for missing client secret.
func TestOAuthRefreshMissingSecret(t *testing.T) {
	t.Parallel()

	_, err := client.NewOAuthRefreshAuthenticator("id", "", "refresh")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
	if !contains(err.Error(), "oauth_client_secret") {
		t.Errorf("expected error to mention 'oauth_client_secret', got: %s", err.Error())
	}
}

// TestOAuthRefreshMissingRefreshToken verifies clear error for missing refresh token.
func TestOAuthRefreshMissingRefreshToken(t *testing.T) {
	t.Parallel()

	_, err := client.NewOAuthRefreshAuthenticator("id", "secret", "")
	if err == nil {
		t.Fatal("expected error for missing refresh token")
	}
	if !contains(err.Error(), "oauth_refresh_token") {
		t.Errorf("expected error to mention 'oauth_refresh_token', got: %s", err.Error())
	}
}

// TestOAuthClientCredentialsMissingClientID verifies clear error for missing client ID.
func TestOAuthClientCredentialsMissingClientID(t *testing.T) {
	t.Parallel()

	_, err := client.NewOAuthClientCredentialsAuthenticator("", "secret")
	if err == nil {
		t.Fatal("expected error for missing client ID")
	}
}

// TestOAuthClientCredentialsMissingSecret verifies clear error for missing secret.
func TestOAuthClientCredentialsMissingSecret(t *testing.T) {
	t.Parallel()

	_, err := client.NewOAuthClientCredentialsAuthenticator("id", "")
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

// TestOAuthRefreshValidCreation verifies valid creation succeeds.
func TestOAuthRefreshValidCreation(t *testing.T) {
	t.Parallel()

	auth, err := client.NewOAuthRefreshAuthenticator("id", "secret", "refresh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil authenticator")
	}
}

// TestOAuthClientCredentialsValidCreation verifies valid creation succeeds.
func TestOAuthClientCredentialsValidCreation(t *testing.T) {
	t.Parallel()

	auth, err := client.NewOAuthClientCredentialsAuthenticator("id", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if auth == nil {
		t.Fatal("expected non-nil authenticator")
	}
}
