package vaultwarden

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// AuthMethod represents the type of authentication being used
type AuthMethod int

const (
	AuthMethodNone AuthMethod = iota
	AuthMethodUser
	AuthMethodAdmin
)

// Client represents a Vaultwarden API client
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client

	// Auth-related fields
	email          string
	masterPassword string
	adminToken     string

	// Runtime auth state
	accessToken string
	privateKey  *rsa.PrivateKey
	adminCookie *http.Cookie

	// Mutex for thread-safe token refresh
	mu sync.RWMutex
}

// New creates a new Vaultwarden API client
func New(baseURL string, opts ...Option) (*Client, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	client := &Client{
		baseURL: parsedURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, fmt.Errorf("error applying client option: %w", err)
		}
	}

	return client, nil
}

// GetEmail returns the email of the authenticated user
func (c *Client) GetEmail() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.email
}

// doRequest performs an HTTP request with automatic authentication handling and JSON processing
func (c *Client) doRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) error {
	// Ensure we have valid authentication for this request
	if err := c.ensureValidAuth(ctx, path); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Prepare request body if provided
	var bodyReader io.Reader
	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	// Create the request
	reqURL := c.baseURL.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add auth based on the path
	authMethod := c.requiresAuth(path)
	switch authMethod {
	case AuthMethodUser:
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	case AuthMethodAdmin:
		if c.adminCookie != nil {
			req.AddCookie(c.adminCookie)
		}
	default:
		return fmt.Errorf("unknown auth method for path: %s", path)
	}

	// Send the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle auth errors and retry once if needed
	if resp.StatusCode == http.StatusUnauthorized {
		// Clear the current auth state
		c.mu.Lock()
		if authMethod == AuthMethodUser {
			c.accessToken = ""
		} else if authMethod == AuthMethodAdmin {
			c.adminCookie = nil
		}
		c.mu.Unlock()

		// Retry the request once
		return c.doRequest(ctx, method, path, reqBody, respBody)
	}

	// Handle error responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		vwError := &models.VaultwardenError{Path: path}

		// Try to parse as admin error first if it's an admin path
		if strings.HasPrefix(path, "/admin") {
			var adminErr models.AdminError
			if err := json.Unmarshal(body, &adminErr); err == nil {
				vwError.AdminError = &adminErr
				return vwError
			}
		}

		// Try to parse as API error
		var apiErr models.APIError
		if err := json.Unmarshal(body, &apiErr); err == nil {
			vwError.APIError = &apiErr
			return vwError
		}

		// If we can't parse either error type, return a generic error
		return fmt.Errorf("request failed with status %d: %s. Response: %s",
			resp.StatusCode, resp.Status, string(body))
	}

	// Parse successful response if a response struct is provided
	if respBody != nil && len(body) > 0 {
		if err := json.Unmarshal(body, respBody); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// doUnauthenticatedRequest performs an HTTP request without authentication
func (c *Client) doUnauthenticatedRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) error {
	var bodyReader io.Reader

	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	// Create the request
	reqURL := c.baseURL.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type for JSON requests
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle error responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request failed with status %d: %s. Response: %s",
			resp.StatusCode, resp.Status, string(body))
	}

	// Parse successful response if a response struct is provided
	if respBody != nil && len(body) > 0 {
		if err := json.Unmarshal(body, respBody); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Get performs a GET request
func (c *Client) Get(ctx context.Context, path string, response interface{}) error {
	return c.doRequest(ctx, http.MethodGet, path, nil, response)
}

// Post performs a POST request
func (c *Client) Post(ctx context.Context, path string, request, response interface{}) error {
	return c.doRequest(ctx, http.MethodPost, path, request, response)
}

// Put performs a PUT request
func (c *Client) Put(ctx context.Context, path string, request, response interface{}) error {
	return c.doRequest(ctx, http.MethodPut, path, request, response)
}

// Delete performs a DELETE request
func (c *Client) Delete(ctx context.Context, path string, request interface{}) error {
	return c.doRequest(ctx, http.MethodDelete, path, request, nil)
}

// DeleteWithResponse performs a DELETE request and parses the response
func (c *Client) DeleteWithResponse(ctx context.Context, path string, request, response interface{}) error {
	return c.doRequest(ctx, http.MethodDelete, path, request, response)
}
