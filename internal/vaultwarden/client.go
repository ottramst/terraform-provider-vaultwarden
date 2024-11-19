package vaultwarden

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultDeviceType = "21"
	DefaultDeviceName = "Vaultwarden_Terraform_Provider"
)

// DeviceInfo holds information about the client device
type DeviceInfo struct {
	DeviceType       string
	DeviceIdentifier string
	DeviceName       string
}

// Client represents a Vaultwarden API client
type Client struct {
	endpoint   *url.URL
	httpClient *http.Client

	// Auth credentials
	Credentials    *models.Credentials
	userAuthMethod AuthMethod

	// Authenticated state
	AuthState *AuthState

	// Device info
	DeviceInfo *DeviceInfo
}

// New creates a new Vaultwarden client with the given endpoint and options
func New(endpoint string, opts ...ClientOption) (*Client, error) {
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	// Generate device identifier
	deviceID := uuid.New().String()

	client := &Client{
		endpoint: parsedURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		DeviceInfo: &DeviceInfo{
			DeviceType:       DefaultDeviceType,
			DeviceIdentifier: deviceID,
			DeviceName:       DefaultDeviceName,
		},
		Credentials: &models.Credentials{},
	}

	// Apply any provided options
	for _, opt := range opts {
		if err := opt(client); err != nil {
			return nil, fmt.Errorf("failed to apply client option: %w", err)
		}
	}

	// Validate credentials
	if err := client.validateCredentials(); err != nil {
		return nil, fmt.Errorf("failed to validate credentials: %w", err)
	}

	return client, nil
}

// prepareRequestBody prepares the request body and returns the appropriate content type
func prepareRequestBody(reqBody interface{}) (io.Reader, string, error) {
	if reqBody == nil {
		return nil, "", nil
	}

	var bodyReader io.Reader
	var contentType string

	switch v := reqBody.(type) {
	case url.Values:
		// Handle form-encoded data
		bodyReader = strings.NewReader(v.Encode())
		contentType = "application/x-www-form-urlencoded"
	case string:
		// Handle raw string data
		bodyReader = strings.NewReader(v)
	case []byte:
		// Handle raw byte data
		bodyReader = bytes.NewReader(v)
	default:
		// Handle JSON data
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
		contentType = "application/json"
	}

	return bodyReader, contentType, nil
}

// doUnauthenticatedRequest performs a request without authentication
//
//nolint:unparam
func (c *Client) doUnauthenticatedRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) (*http.Response, error) {
	// Prepare request body
	bodyReader, contentType, err := prepareRequestBody(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request body: %w", err)
	}

	// Create request with context
	reqURL := c.endpoint.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type if body is present
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle error responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, fmt.Errorf("request failed with status %d: %s. Response: %s",
			resp.StatusCode, resp.Status, string(body))
	}

	// Parse successful response if a response struct is provided
	if respBody != nil && len(body) > 0 {
		if err := json.Unmarshal(body, respBody); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp, nil
}

// doRequest performs a request with appropriate authentication
//
//nolint:unparam
func (c *Client) doRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) (*http.Response, error) {
	// Prepare request body
	bodyReader, contentType, err := prepareRequestBody(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request body: %w", err)
	}

	// Create request with context
	reqURL := c.endpoint.JoinPath(path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type if body is present
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Add authentication to request
	if err := c.authenticateRequest(req); err != nil {
		return nil, fmt.Errorf("failed to authenticate request: %w", err)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle error responses
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, fmt.Errorf("request failed with status %d: %s. Response: %s",
			resp.StatusCode, resp.Status, string(body))
	}

	// Parse successful response if a response struct is provided
	if respBody != nil && len(body) > 0 {
		if err := json.Unmarshal(body, respBody); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp, nil
}
