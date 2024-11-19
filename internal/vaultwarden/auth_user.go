package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/helpers"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
	"net/url"
	"time"
)

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
}

// ensureUserAuth ensures that user authentication is valid
func (c *Client) ensureUserAuth(ctx context.Context) error {
	// Check if we have a valid user session
	if c.AuthState != nil && c.AuthState.AccessToken != "" && c.AuthState.PrivateKey != nil {
		// Check if token is not expired (with some buffer time)
		if !c.AuthState.TokenExpiresAt.IsZero() && time.Now().Add(time.Minute).Before(c.AuthState.TokenExpiresAt) {
			return nil
		}
	}

	// Perform user login
	return c.userLogin(ctx)
}

// userLogin performs the user authentication
func (c *Client) userLogin(ctx context.Context) error {
	// 1. Get KDF configuration if not already present
	if c.AuthState == nil || c.AuthState.KdfConfig == nil {
		preloginResp, err := c.PreLogin(ctx)
		if err != nil {
			return fmt.Errorf("failed to get prelogin info: %w", err)
		}

		// Build the KDF configuration
		kdfConfig := &models.KdfConfiguration{
			KdfType:        preloginResp.Kdf,
			KdfIterations:  preloginResp.KdfIterations,
			KdfMemory:      preloginResp.KdfMemory,
			KdfParallelism: preloginResp.KdfParallelism,
		}

		if c.AuthState == nil {
			c.AuthState = &AuthState{}
		}
		c.AuthState.KdfConfig = kdfConfig
	}

	// 2. Build a prelogin key
	preloginKey, err := keybuilder.BuildPreloginKey(c.Credentials.MasterPassword, c.Credentials.Email, c.AuthState.KdfConfig)
	if err != nil {
		return fmt.Errorf("failed to build prelogin key: %w", err)
	}

	// 3. Hash the password
	hashedPassword := crypt.HashPassword(c.Credentials.MasterPassword, *preloginKey, false)

	// 4. Perform the login request
	var tokenResp *TokenResponse
	switch c.userAuthMethod {
	case AuthMethodOAuth2:
		tokenResp, err = c.loginWithAPIKey(ctx)
		if err != nil {
			return fmt.Errorf("API key authentication failed: %w", err)
		}
	case AuthMethodUserPassword:
		tokenResp, err = c.LoginWithUserCredentials(ctx, hashedPassword)
		if err != nil {
			return fmt.Errorf("user credential authentication failed: %w", err)
		}
	default:
		return fmt.Errorf("no valid user authentication method available")
	}

	// 5. Decrypt the encryption key
	encryptionKey, err := crypt.DecryptEncryptionKey(tokenResp.Key, *preloginKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt encryption key: %w", err)
	}

	// 6. Decrypt the private key
	privateKey, err := crypt.DecryptPrivateKey(tokenResp.PrivateKey, *encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt private key: %w", err)
	}

	// Parse token expiration
	expirationTime, err := helpers.ParseJWTExpiration(tokenResp.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to parse token expiration: %w", err)
	}

	// Update auth state
	c.AuthState.AccessToken = tokenResp.AccessToken
	c.AuthState.PrivateKey = privateKey
	c.AuthState.TokenExpiresAt = expirationTime

	return nil
}

// preloginRequest represents the request body for the prelogin endpoint
type preloginRequest struct {
	Email string `json:"email"`
}

// PreloginResponse represents the response from the prelogin endpoint
type PreloginResponse struct {
	Kdf            models.KdfType `json:"kdf"`
	KdfIterations  int            `json:"kdfIterations"`
	KdfMemory      int            `json:"kdfMemory"`
	KdfParallelism int            `json:"kdfParallelism"`
}

// PreLogin retrieves KDF configuration for the given email
func (c *Client) PreLogin(ctx context.Context) (*PreloginResponse, error) {
	// Prepare request body
	reqBody := preloginRequest{
		Email: c.Credentials.Email,
	}

	// Make request
	var preloginResp PreloginResponse
	resp, err := c.doUnauthenticatedRequest(ctx, http.MethodPost, "/identity/accounts/prelogin", reqBody, &preloginResp)
	if err != nil {
		return nil, fmt.Errorf("prelogin request failed: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prelogin failed with status code: %d", resp.StatusCode)
	}

	return &preloginResp, nil
}

func (c *Client) LoginWithUserCredentials(ctx context.Context, hashedPassword string) (*TokenResponse, error) {
	// Prepare request body
	form := url.Values{}
	form.Add("scope", "api offline_access")
	form.Add("client_id", "cli")
	form.Add("grant_type", "password")
	form.Add("username", c.Credentials.Email)
	form.Add("password", hashedPassword)
	form.Add("deviceType", c.DeviceInfo.DeviceType)
	form.Add("deviceIdentifier", c.DeviceInfo.DeviceIdentifier)
	form.Add("deviceName", c.DeviceInfo.DeviceName)

	var tokenResp TokenResponse
	if _, err := c.doUnauthenticatedRequest(ctx, http.MethodPost, "/identity/connect/token", form, &tokenResp); err != nil {
		return nil, fmt.Errorf("user credential authentication failed: %w", err)
	}

	return &tokenResp, nil
}

func (c *Client) loginWithAPIKey(ctx context.Context) (*TokenResponse, error) {
	// Prepare request body
	form := url.Values{}
	form.Add("scope", "api")
	form.Add("client_id", c.Credentials.ClientID)
	form.Add("client_secret", c.Credentials.ClientSecret)
	form.Add("grant_type", "client_credentials")
	form.Add("deviceType", c.DeviceInfo.DeviceType)
	form.Add("deviceIdentifier", c.DeviceInfo.DeviceIdentifier)
	form.Add("deviceName", c.DeviceInfo.DeviceName)

	var tokenResp TokenResponse
	if _, err := c.doUnauthenticatedRequest(ctx, http.MethodPost, "/identity/connect/token", form, &tokenResp); err != nil {
		return nil, fmt.Errorf("API key authentication failed: %w", err)
	}

	return &tokenResp, nil
}
