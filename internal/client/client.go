// Package client provides the Atlassian Cloud API client.
//
// This is a purpose-built thin client with zero third-party Atlassian SDK
// dependencies. It handles pagination transparency, rate-limit resilience
// with exponential backoff and jitter, and error translation to clear
// user-facing messages.
//
// The client supports two base URLs: one for site-specific APIs
// (e.g., https://site.atlassian.net) and one for the Atlassian Admin API
// (e.g., https://api.atlassian.com). Site-specific operations use Get/Post/Put/Delete;
// admin operations use AdminGet/AdminPost.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config holds the configuration for the Atlassian API client.
type Config struct {
	BaseURL        string
	AdminBaseURL   string
	RequestTimeout time.Duration
	MaxRetries     int
	RetryWaitMin   time.Duration
	RetryWaitMax   time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		AdminBaseURL:   "https://api.atlassian.com",
		RequestTimeout: 30 * time.Second,
		MaxRetries:     5,
		RetryWaitMin:   1 * time.Second,
		RetryWaitMax:   30 * time.Second,
	}
}

// Authenticator is the interface for authentication strategies.
type Authenticator interface {
	// AuthenticateRequest adds authentication headers to an HTTP request.
	AuthenticateRequest(req *http.Request) error
}

// Client is the Atlassian Cloud API client.
type Client struct {
	httpClient *http.Client
	config     Config
	auth       Authenticator
	adminAuth  Authenticator
}

// NewClient creates a new Atlassian API client.
// BaseURL may be empty if only Admin API operations are needed.
func NewClient(config Config, auth Authenticator) (*Client, error) {
	if config.BaseURL != "" {
		parsed, err := url.Parse(config.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("atlassian provider configuration error: invalid URL %q: %w", config.BaseURL, err)
		}
		if parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("atlassian provider configuration error: URL must include scheme and host (e.g., https://example.atlassian.net)")
		}
	}

	if auth == nil {
		return nil, fmt.Errorf("atlassian provider configuration error: no authentication method configured. " +
			"Configure either an API key (api_key + username) or OAuth 2.0 (oauth_client_id + oauth_client_secret)")
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
		},
		config:    config,
		auth:      auth,
		adminAuth: auth,
	}, nil
}

// SetAdminAuth sets a separate authenticator for Admin API calls.
func (c *Client) SetAdminAuth(auth Authenticator) {
	c.adminAuth = auth
}

// SetAdminBaseURL configures the Admin API base URL after client creation.
func (c *Client) SetAdminBaseURL(adminURL string) {
	c.config.AdminBaseURL = adminURL
}

// APIError represents a structured error from the Atlassian API.
type APIError struct {
	StatusCode int
	Message    string
	Resource   string
	Action     string
}

// Error returns a clear, user-friendly error message.
func (e *APIError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("atlassian API error (HTTP %d)", e.StatusCode))
	if e.Resource != "" {
		sb.WriteString(fmt.Sprintf(" on %s", e.Resource))
	}
	if e.Action != "" {
		sb.WriteString(fmt.Sprintf(" during %s", e.Action))
	}
	sb.WriteString(fmt.Sprintf(": %s", e.Message))
	return sb.String()
}

// Do executes an HTTP request against the site-specific API with authentication,
// retry logic, and error translation.
func (c *Client) Do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if c.config.BaseURL == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: 'url' is required for site-specific API calls. " +
			"Set the 'url' attribute in the provider block or the ATLASSIAN_URL environment variable")
	}
	return c.doWithBase(ctx, method, c.config.BaseURL, path, body, c.auth)
}

// AdminDo executes an HTTP request against the Atlassian Admin API with authentication,
// retry logic, and error translation.
func (c *Client) AdminDo(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if c.config.AdminBaseURL == "" {
		return nil, fmt.Errorf("atlassian provider configuration error: admin API URL is not configured. " +
			"Set the 'admin_url' attribute in the provider block or the ATLASSIAN_ADMIN_URL environment variable")
	}
	return c.doWithBase(ctx, method, c.config.AdminBaseURL, path, body, c.adminAuth)
}

// doWithBase executes an HTTP request against the specified base URL with authentication,
// retry logic, and error translation.
func (c *Client) doWithBase(ctx context.Context, method, baseURL, path string, body io.Reader, auth Authenticator) (*http.Response, error) {
	fullURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")

	var lastErr error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			wait := c.backoffWithJitter(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		if err := auth.AuthenticateRequest(req); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request to %s failed: %w", path, err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			resp.Body.Close()
			lastErr = &APIError{
				StatusCode: resp.StatusCode,
				Message:    "rate limited by Atlassian API, retrying",
			}
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request to %s failed after %d retries: %w", path, c.config.MaxRetries, lastErr)
}

// Get performs an authenticated GET request against the site API and returns the parsed response.
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	resp, err := c.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.translateError(resp, path, "read")
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// Post performs an authenticated POST request against the site API.
func (c *Client) Post(ctx context.Context, path string, body io.Reader, result interface{}) error {
	resp, err := c.Do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.translateError(resp, path, "create")
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// Put performs an authenticated PUT request against the site API.
func (c *Client) Put(ctx context.Context, path string, body io.Reader, result interface{}) error {
	resp, err := c.Do(ctx, http.MethodPut, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.translateError(resp, path, "update")
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// Delete performs an authenticated DELETE request against the site API.
func (c *Client) Delete(ctx context.Context, path string) error {
	resp, err := c.Do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.translateError(resp, path, "delete")
	}
	return nil
}

// AdminGet performs an authenticated GET request against the Atlassian Admin API.
func (c *Client) AdminGet(ctx context.Context, path string, result interface{}) error {
	resp, err := c.AdminDo(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.translateError(resp, path, "read")
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// AdminPost performs an authenticated POST request against the Atlassian Admin API.
func (c *Client) AdminPost(ctx context.Context, path string, body io.Reader, result interface{}) error {
	resp, err := c.AdminDo(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.translateError(resp, path, "create")
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}
	return nil
}

// GetPaginated fetches all pages of a paginated endpoint and returns the complete result set.
// The caller receives all items; pagination details are never exposed.
func (c *Client) GetPaginated(ctx context.Context, path string, extractItems func(json.RawMessage) ([]json.RawMessage, string, error)) ([]json.RawMessage, error) {
	var allItems []json.RawMessage
	currentPath := path

	for currentPath != "" {
		var raw json.RawMessage
		if err := c.Get(ctx, currentPath, &raw); err != nil {
			return nil, err
		}

		items, nextPath, err := extractItems(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to extract paginated results: %w", err)
		}

		allItems = append(allItems, items...)
		currentPath = nextPath
	}

	return allItems, nil
}

// backoffWithJitter calculates the wait time using exponential backoff with full jitter.
func (c *Client) backoffWithJitter(attempt int) time.Duration {
	base := float64(c.config.RetryWaitMin)
	max := float64(c.config.RetryWaitMax)
	exp := base * math.Pow(2, float64(attempt-1))
	if exp > max {
		exp = max
	}
	jittered := time.Duration(rand.Float64() * exp)
	if jittered < c.config.RetryWaitMin {
		jittered = c.config.RetryWaitMin
	}
	return jittered
}

// translateError converts raw Atlassian API error responses to clear user-facing messages.
func (c *Client) translateError(resp *http.Response, resource, action string) error {
	bodyBytes, _ := io.ReadAll(resp.Body)

	// Try to parse Atlassian's standard error format
	var apiResp struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
		Message       string            `json:"message"`
	}

	message := http.StatusText(resp.StatusCode)
	if json.Unmarshal(bodyBytes, &apiResp) == nil {
		if apiResp.Message != "" {
			message = apiResp.Message
		} else if len(apiResp.ErrorMessages) > 0 {
			message = strings.Join(apiResp.ErrorMessages, "; ")
		} else if len(apiResp.Errors) > 0 {
			var parts []string
			for field, msg := range apiResp.Errors {
				parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
			}
			message = strings.Join(parts, "; ")
		}
	}

	// Add helpful context based on status code
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		message += ". Check your authentication credentials (API token or OAuth configuration)"
	case http.StatusForbidden:
		message += ". The authenticated user does not have permission for this operation"
	case http.StatusNotFound:
		message += ". Verify the resource exists and the URL is correct"
	case http.StatusConflict:
		message += ". A resource with this identifier may already exist"
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    message,
		Resource:   resource,
		Action:     action,
	}
}
