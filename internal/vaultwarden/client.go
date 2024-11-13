package vaultwarden

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Client represents a Vaultwarden API client
type Client struct {
	baseURL    string
	httpClient *http.Client
	adminToken string
}

// Option allows for customizing the client
type Option func(*Client)

// New creates a new Vaultwarden API client
func New(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	// Ensure baseURL doesn't end with a slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	// Create cookie jar for session cookies
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

// WithHTTPClient allows setting a custom HTTP client
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithAdminToken sets the admin token for the client
func WithAdminToken(token string) Option {
	return func(c *Client) {
		c.adminToken = token
	}
}

// hasValidCookie checks if there's a valid admin session cookie
func (c *Client) hasValidCookie() bool {
	parsedURL, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}

	for _, cookie := range c.httpClient.Jar.Cookies(parsedURL) {
		if cookie.Name == "VW_ADMIN" &&
			cookie.Value != "" &&
			(cookie.Expires.IsZero() || cookie.Expires.After(time.Now())) {
			return true
		}
	}
	return false
}

// adminLogin performs the admin authentication to get the session cookie
func (c *Client) adminLogin() error {
	if c.adminToken == "" {
		return fmt.Errorf("admin token is required for admin operations")
	}

	if c.hasValidCookie() {
		return nil
	}

	formData := url.Values{}
	formData.Set("token", c.adminToken)

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/admin", c.baseURL),
		strings.NewReader(formData.Encode()),
	)
	if err != nil {
		return fmt.Errorf("failed to create auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth failed with status %d: %s. Response: %s",
			resp.StatusCode, resp.Status, string(body))
	}

	return nil
}

// AdminRequest performs an authenticated request to the admin API
func (c *Client) AdminRequest(method string, path string, body interface{}) (*http.Response, error) {
	return c.AdminRequestWithContext(context.Background(), method, path, body)
}

// AdminRequestWithContext performs an authenticated request to the admin API with context
func (c *Client) AdminRequestWithContext(ctx context.Context, method string, path string, body interface{}) (*http.Response, error) {
	// Ensure we have a valid session
	if err := c.adminLogin(); err != nil {
		return nil, fmt.Errorf("admin authentication failed: %w", err)
	}

	// Ensure path starts with /admin/
	if !strings.HasPrefix(path, "/admin/") {
		path = "/admin/" + strings.TrimPrefix(path, "/")
	}

	var bodyReader bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&bodyReader).Encode(body); err != nil {
			return nil, fmt.Errorf("failed to encode request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	return resp, nil
}

// InviteUserRequest represents the request body for inviting a user
type InviteUserRequest struct {
	Email string `json:"email"`
}

// InviteUser invites a new user to Vaultwarden
func (c *Client) InviteUser(email string) (*models.User, error) {
	// Validate email format
	if _, err := mail.ParseAddress(email); err != nil {
		return nil, fmt.Errorf("invalid email format: %s", email)
	}

	// Print request details for debugging
	fmt.Printf("Inviting user with email: %s\n", email)

	req := InviteUserRequest{Email: email}
	resp, err := c.AdminRequest(http.MethodPost, "/admin/invite", req)
	if err != nil {
		return nil, fmt.Errorf("failed to invite user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, fmt.Errorf("user already exists with email: %s", email)
	}

	var user models.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode invite response: %w", err)
	}

	return &user, nil
}

// GetUser retrieves a user by their ID
func (c *Client) GetUser(ID string) (*models.User, *http.Response, error) {
	resp, err := c.AdminRequest(http.MethodGet, fmt.Sprintf("/admin/users/%s", ID), nil)
	if err != nil {
		return nil, resp, fmt.Errorf("failed to get user: %w", err)
	}
	defer resp.Body.Close()

	var user models.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, resp, fmt.Errorf("failed to decode user response: %w", err)
	}

	return &user, resp, nil
}

// DeleteUser deletes a user by their ID
func (c *Client) DeleteUser(ID string) error {
	resp, err := c.AdminRequest(http.MethodPost, fmt.Sprintf("/admin/users/%s/delete", ID), nil)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
