package vaultwarden

import (
	"context"
	"crypto/rsa"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/helpers"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
	"net/http"
	"net/url"
	"time"
)

// TokenResponse represents the response from the login endpoint.
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

// ensureUserAuth ensures that user authentication is valid.
func (c *Client) ensureUserAuth(ctx context.Context) error {
	_, _, err := c.userSession(ctx)
	return err
}

// userSession returns the access token and private key of a valid user session, logging in
// first if needed. Concurrent callers wait for a single login.
func (c *Client) userSession(ctx context.Context) (string, *rsa.PrivateKey, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if err := c.ensureUserSessionLocked(ctx); err != nil {
		return "", nil, err
	}

	return c.AuthState.AccessToken, c.AuthState.PrivateKey, nil
}

// UserKey returns the symmetric key of the provider user, which encrypts the personal vault,
// logging in first if needed.
func (c *Client) UserKey(ctx context.Context) (symmetrickey.Key, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	if err := c.ensureUserSessionLocked(ctx); err != nil {
		return symmetrickey.Key{}, err
	}

	return *c.AuthState.UserKey, nil
}

// ensureUserSessionLocked logs in unless the current user session is valid. The caller must
// hold authMu.
func (c *Client) ensureUserSessionLocked(ctx context.Context) error {
	// Check if we have a valid user session
	if c.AuthState != nil && c.AuthState.AccessToken != "" && c.AuthState.PrivateKey != nil && c.AuthState.UserKey != nil {
		// Check if token is not expired (with some buffer time)
		if !c.AuthState.TokenExpiresAt.IsZero() && time.Now().Add(time.Minute).Before(c.AuthState.TokenExpiresAt) {
			return nil
		}
	}

	return c.userLogin(ctx)
}

// userLogin performs the user authentication. The caller must hold authMu.
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

	// Fetch the user profile with the new token. GetProfile would wait for this login.
	var user models.User
	authorize := func(req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
		return nil
	}
	if _, err := c.send(ctx, http.MethodGet, "/api/accounts/profile", nil, &user, requestOptions{authorize: authorize}); err != nil {
		return fmt.Errorf("failed to get user profile: %w", err)
	}

	// Decrypt the organization keys
	organizations, err := decryptOrganizationSecrets(&user, privateKey)
	if err != nil {
		return err
	}

	// Update auth state
	c.AuthState.AccessToken = tokenResp.AccessToken
	c.AuthState.PrivateKey = privateKey
	c.AuthState.UserKey = encryptionKey
	c.AuthState.UserID = user.ID
	c.AuthState.TokenExpiresAt = expirationTime

	c.orgMu.Lock()
	c.AuthState.Organizations = organizations
	c.orgMu.Unlock()

	return nil
}

// decryptOrganizationSecrets decrypts the keys of the organizations in the user's profile.
func decryptOrganizationSecrets(user *models.User, privateKey *rsa.PrivateKey) (map[string]OrganizationSecret, error) {
	organizations := make(map[string]OrganizationSecret)
	for _, org := range user.Organizations {
		if !org.Enabled || org.Key == "" {
			continue
		}

		// Decrypt the organization key
		decryptedKeyBytes, err := keybuilder.RSADecrypt(org.Key, privateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt organization key for org %s: %w", org.ID, err)
		}

		// Convert decrypted key to symmetrickey.Key
		decryptedKey, err := symmetrickey.NewFromRawBytes(decryptedKeyBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to construct symmetric key for org %s: %w", org.ID, err)
		}

		organizations[org.ID] = OrganizationSecret{
			Key:              *decryptedKey,
			OrganizationUUID: org.ID,
			Name:             org.Name,
		}
	}
	return organizations, nil
}

// refreshOrganizationSecrets reads the organization keys from the profile again, to get the keys
// of organizations that the user joined, or that another client created, since the login.
func (c *Client) refreshOrganizationSecrets(ctx context.Context) error {
	_, privateKey, err := c.userSession(ctx)
	if err != nil {
		return err
	}
	profile, err := c.GetProfile(ctx)
	if err != nil {
		return err
	}
	secrets, err := decryptOrganizationSecrets(profile, privateKey)
	if err != nil {
		return err
	}
	for _, secret := range secrets {
		c.setOrganizationSecret(secret)
	}
	return nil
}

// OrganizationSecret returns the cached key of an organization the user belongs to.
func (c *Client) OrganizationSecret(orgID string) (OrganizationSecret, bool) {
	c.orgMu.RLock()
	defer c.orgMu.RUnlock()

	if c.AuthState == nil {
		return OrganizationSecret{}, false
	}
	secret, ok := c.AuthState.Organizations[orgID]
	return secret, ok
}

// setOrganizationSecret caches the key of an organization.
func (c *Client) setOrganizationSecret(secret OrganizationSecret) {
	c.orgMu.Lock()
	defer c.orgMu.Unlock()

	if c.AuthState.Organizations == nil {
		c.AuthState.Organizations = make(map[string]OrganizationSecret)
	}
	c.AuthState.Organizations[secret.OrganizationUUID] = secret
}

// preloginRequest represents the request body for the prelogin endpoint.
type preloginRequest struct {
	Email string `json:"email"`
}

// PreloginResponse represents the response from the prelogin endpoint.
type PreloginResponse struct {
	Kdf            models.KdfType `json:"kdf"`
	KdfIterations  int            `json:"kdfIterations"`
	KdfMemory      int            `json:"kdfMemory"`
	KdfParallelism int            `json:"kdfParallelism"`
}

// PreLogin retrieves KDF configuration for the given email.
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
