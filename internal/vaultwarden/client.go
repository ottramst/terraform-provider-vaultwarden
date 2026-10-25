package vaultwarden

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDeviceType = "21"
	DefaultDeviceName = "Vaultwarden_Terraform_Provider"
)

// DeviceInfo holds information about the client device.
type DeviceInfo struct {
	DeviceType       string
	DeviceIdentifier string
	DeviceName       string
}

// Client represents a Vaultwarden API client.
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

	// authMu serializes logins and guards the session in AuthState
	authMu sync.Mutex
	// orgMu guards AuthState.Organizations
	orgMu sync.RWMutex

	// maxRetryWait is the total time a request waits for retries when it is rate limited
	maxRetryWait time.Duration
	// retryBaseDelay is the wait before the first retry; it doubles on each retry
	retryBaseDelay time.Duration

	// accessLocks serializes changes to collection assignments per organization, see lockOrganizationAccess
	accessLocksMu sync.Mutex
	accessLocks   map[string]*sync.Mutex
}

// lockOrganizationAccess locks changes to the collection assignments of an organization and
// returns the unlock function. Vaultwarden only offers endpoints that replace all assignments
// of a collection or member, so concurrent read-modify-write updates would overwrite each other.
func (c *Client) lockOrganizationAccess(orgID string) func() {
	c.accessLocksMu.Lock()
	if c.accessLocks == nil {
		c.accessLocks = make(map[string]*sync.Mutex)
	}
	mutex, ok := c.accessLocks[orgID]
	if !ok {
		mutex = &sync.Mutex{}
		c.accessLocks[orgID] = mutex
	}
	c.accessLocksMu.Unlock()

	mutex.Lock()
	return mutex.Unlock
}

// New creates a new Vaultwarden client with the given endpoint and options.
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
		Credentials:    &models.Credentials{},
		maxRetryWait:   DefaultMaxRetryWait,
		retryBaseDelay: time.Second,
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

// requestURL returns the URL for path, which may include a query string.
func (c *Client) requestURL(path string) *url.URL {
	path, query, _ := strings.Cut(path, "?")
	reqURL := c.endpoint.JoinPath(path)
	reqURL.RawQuery = query
	return reqURL
}

// prepareRequestBody prepares the request body and returns the appropriate content type.
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

// requestOptions configures how send performs a request.
type requestOptions struct {
	// authorize adds authentication to the request, if set
	authorize func(*http.Request) error
	// noRedirects returns redirect responses instead of following them
	noRedirects bool
}

// doUnauthenticatedRequest performs a request without authentication
//
//nolint:unparam
func (c *Client) doUnauthenticatedRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) (*http.Response, error) {
	return c.send(ctx, method, path, reqBody, respBody, requestOptions{})
}

// doRequest performs a request with appropriate authentication
//
//nolint:unparam
func (c *Client) doRequest(ctx context.Context, method, path string, reqBody, respBody interface{}) (*http.Response, error) {
	return c.send(ctx, method, path, reqBody, respBody, requestOptions{authorize: c.authenticateRequest})
}

// DefaultMaxRetryWait is the default total time a rate limited request waits for retries.
const DefaultMaxRetryWait = time.Minute

// maxRetryDelay caps the wait between two retries.
const maxRetryDelay = 30 * time.Second

// send performs a request and decodes a JSON response into respBody, if set. Rate limited
// requests (HTTP 429) are retried with exponential backoff for up to maxRetryWait in total.
// Vaultwarden sends no Retry-After header, so the wait is not taken from the response.
func (c *Client) send(ctx context.Context, method, path string, reqBody, respBody interface{}, opts requestOptions) (*http.Response, error) {
	delay := c.retryBaseDelay
	var waited time.Duration

	for {
		resp, err := c.sendOnce(ctx, method, path, reqBody, respBody, opts)

		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
			return resp, err
		}

		// Wait between half and all of the delay, so concurrent requests spread out
		wait := delay/2 + time.Duration(rand.Int64N(int64(delay/2)+1))
		if waited+wait > c.maxRetryWait {
			return resp, fmt.Errorf("%w\n\nVaultwarden rate limited the request, and it still failed after waiting %s for retries. %s",
				err, waited.Round(time.Second), rateLimitHint(apiErr.Body))
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return resp, err
		case <-timer.C:
		}
		waited += wait
		delay = min(delay*2, maxRetryDelay)
	}
}

// rateLimitHint names the Vaultwarden settings of the rate limit that a 429 response body came from.
func rateLimitHint(body string) string {
	switch {
	case strings.Contains(body, "Too many login requests"):
		return "Raise the login rate limit (LOGIN_RATELIMIT_SECONDS, LOGIN_RATELIMIT_MAX_BURST) on the server, or reduce the number of provider instances logging in."
	case strings.Contains(body, "<html"), strings.Contains(body, "<!DOCTYPE"):
		return "Raise the admin rate limit (ADMIN_RATELIMIT_SECONDS, ADMIN_RATELIMIT_MAX_BURST) on the server."
	default:
		return "Raise the rate limit for unauthenticated requests (UNAUTHENTICATED_RATELIMIT_SECONDS, UNAUTHENTICATED_RATELIMIT_MAX_BURST) on the server."
	}
}

// sendOnce performs a single attempt of a request, see send.
func (c *Client) sendOnce(ctx context.Context, method, path string, reqBody, respBody interface{}, opts requestOptions) (*http.Response, error) {
	// Prepare request body
	bodyReader, contentType, err := prepareRequestBody(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request body: %w", err)
	}

	// Create request with context
	reqURL := c.requestURL(path)
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type if body is present
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")

	// Add authentication to request
	if opts.authorize != nil {
		if err := opts.authorize(req); err != nil {
			return nil, fmt.Errorf("failed to authenticate request: %w", err)
		}
	}

	httpClient := c.httpClient
	if opts.noRedirects {
		httpClient = &http.Client{
			Transport: c.httpClient.Transport,
			Timeout:   c.httpClient.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	// Send request
	resp, err := httpClient.Do(req)
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
		return resp, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(body)}
	}

	// Parse successful response if a response struct is provided
	if respBody != nil && len(body) > 0 {
		if err := json.Unmarshal(body, respBody); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return resp, nil
}
