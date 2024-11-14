package vaultwarden

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/helpers"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// hasUserAuth checks if user credentials are configured
func (c *Client) hasUserAuth() bool {
	return c.email != "" && c.masterPassword != ""
}

// hasAdminAuth checks if admin token is configured
func (c *Client) hasAdminAuth() bool {
	return c.adminToken != ""
}

// requiresAuth determines if an endpoint requires authentication and what type
func (c *Client) requiresAuth(path string) AuthMethod {
	if len(path) >= 6 && path[:6] == "/admin" {
		return AuthMethodAdmin
	}

	return AuthMethodUser
}

// PreloginRequest represents the request to the prelogin endpoint
type PreloginRequest struct {
	Email string `json:"email"`
}

// PreloginResponse represents the response from the prelogin endpoint
type PreloginResponse struct {
	Kdf            models.KdfType `json:"kdf"`
	KdfIterations  int            `json:"kdfIterations"`
	KdfMemory      int            `json:"kdfMemory"`
	KdfParallelism int            `json:"kdfParallelism"`
}

// Prelogin performs the Prelogin request
func (c *Client) Prelogin(ctx context.Context) (*PreloginResponse, error) {
	data := PreloginRequest{
		Email: c.email,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		return nil, fmt.Errorf("failed to encode prelogin request: %w", err)
	}

	reqURL := c.baseURL.JoinPath("/identity/accounts/prelogin")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create prelogin request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prelogin request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("prelogin failed with status %d: %s. Response: %s", resp.StatusCode, resp.Status, string(body))
	}

	var preloginResp PreloginResponse
	if err := json.NewDecoder(resp.Body).Decode(&preloginResp); err != nil {
		return nil, fmt.Errorf("failed to decode prelogin response: %w", err)
	}

	return &preloginResp, nil
}

// isTokenValid checks if the token is valid and not expired
func (c *Client) isTokenValid() bool {
	if c.accessToken == "" {
		return false
	}

	claims, err := helpers.ParseJWT(c.accessToken)
	if err != nil {
		return false
	}

	// Check if token is expired or not yet valid
	now := time.Now().Unix()
	if now < claims.NotBefore || now > claims.ExpiresAt {
		return false
	}

	return true
}

// TokenResponse represents the response from the login endpoint
type TokenResponse struct {
	Kdf                 models.KdfType `json:"Kdf"`
	KdfIterations       int            `json:"KdfIterations"`
	KdfMemory           int            `json:"kdfMemory"`
	KdfParallelism      int            `json:"kdfParallelism"`
	Key                 string         `json:"Key"`
	PrivateKey          string         `json:"PrivateKey"`
	ResetMasterPassword bool           `json:"ResetMasterPassword"`
	AccessToken         string         `json:"access_token"`
	ExpireIn            int            `json:"expires_in"`
	RefreshToken        string         `json:"refresh_token"`
	Scope               string         `json:"scope"`
	TokenType           string         `json:"token_type"`
	UnofficialServer    bool           `json:"unofficialServer"`
	RSAPrivateKey       *rsa.PrivateKey
}

// Login performs the login request
func (c *Client) Login(ctx context.Context, hashedPassword string) (*TokenResponse, error) {
	form := url.Values{}
	form.Add("scope", "api offline_access")
	form.Add("client_id", "cli")
	form.Add("grant_type", "password")
	form.Add("username", c.email)
	form.Add("password", hashedPassword)
	form.Add("deviceIdentifier", uuid.New().String())
	form.Add("deviceName", "Vaultwarden_Terraform_Provider")
	form.Add("deviceType", "21")

	reqURL := c.baseURL.JoinPath("/identity/connect/token")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("login request failed: %s", string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode login response: %w", err)
	}

	return &tokenResp, nil
}

// processLoginResponse processes the login response and sets up the client's auth state
func (c *Client) processLoginResponse(tokenResp *TokenResponse, preloginKey *symmetrickey.Key) error {
	// Get the encryption key
	encryptionKey, err := crypt.DecryptEncryptionKey(tokenResp.Key, *preloginKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt encryption key: %w", err)
	}

	// Get the user's private key
	privateKey, err := crypt.DecryptPrivateKey(tokenResp.PrivateKey, *encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt private key: %w", err)
	}

	// Store the auth state
	c.accessToken = tokenResp.AccessToken
	c.privateKey = privateKey

	return nil
}

// ensureValidUserAuth ensures valid user authentication
func (c *Client) ensureValidUserAuth(ctx context.Context) error {
	if !c.hasUserAuth() {
		return fmt.Errorf("user credentials not configured")
	}

	// Check if we have a valid access token
	if c.isTokenValid() && c.privateKey != nil {
		return nil
	}

	// Step 1: Prelogin request
	preloginResp, err := c.Prelogin(ctx)
	if err != nil {
		return fmt.Errorf("prelogin failed: %w", err)
	}

	// Step 2: Build prelogin key

	// Create KDF configuration
	kdfConfig := models.KdfConfiguration{
		KdfType:        preloginResp.Kdf,
		KdfIterations:  preloginResp.KdfIterations,
		KdfMemory:      preloginResp.KdfMemory,
		KdfParallelism: preloginResp.KdfParallelism,
	}

	preloginKey, err := keybuilder.BuildPreloginKey(c.masterPassword, c.email, kdfConfig)
	if err != nil {
		return fmt.Errorf("failed to build prelogin key: %w", err)
	}

	// Step 3: Hash password
	hashedPassword := crypt.HashPassword(c.masterPassword, *preloginKey, false)

	// Step 4: Login request
	tokenResp, err := c.Login(ctx, hashedPassword)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	// Step 5: Process login response
	if err := c.processLoginResponse(tokenResp, preloginKey); err != nil {
		return fmt.Errorf("failed to process login response: %w", err)
	}

	return nil
}

// ensureValidAdminAuth ensures valid admin authentication
func (c *Client) ensureValidAdminAuth(ctx context.Context) error {
	if !c.hasAdminAuth() {
		return fmt.Errorf("admin token not configured")
	}

	// Check if we have a valid admin cookie
	if c.adminCookie != nil && !c.adminCookie.Expires.Before(time.Now()) {
		return nil
	}

	// Create form data
	form := url.Values{}
	form.Add("token", c.adminToken)

	// Create request
	reqURL := c.baseURL.JoinPath("/admin")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create admin auth request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send admin auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("admin auth failed: %s", string(body))
	}

	// Find and store the VW_ADMIN cookie
	cookies := resp.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == "VW_ADMIN" {
			c.adminCookie = cookie
			return nil
		}
	}

	return fmt.Errorf("admin auth successful but no VW_ADMIN cookie received")
}

// ensureValidAuth ensures that the client has valid authentication for the given path
func (c *Client) ensureValidAuth(ctx context.Context, path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	authMethod := c.requiresAuth(path)
	switch authMethod {
	case AuthMethodAdmin:
		return c.ensureValidAdminAuth(ctx)
	case AuthMethodUser:
		return c.ensureValidUserAuth(ctx)
	default:
		return fmt.Errorf("unknown auth method required for %s", path)
	}
}
