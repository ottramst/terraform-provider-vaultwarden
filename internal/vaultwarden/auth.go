package vaultwarden

import (
	"crypto/rsa"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/symmetrickey"
	"net/http"
	"strings"
	"time"
)

// AuthMethod represents the type of authentication being used
type AuthMethod int

const (
	AuthMethodNone AuthMethod = iota
	AuthMethodAdmin
	AuthMethodUserPassword
	AuthMethodOAuth2
)

type OrganizationSecret struct {
	Key              symmetrickey.Key
	OrganizationUUID string
	Name             string
}

// AuthState holds the current authentication state
type AuthState struct {
	// Admin authentication
	AdminCookie *http.Cookie

	// User authentication
	AccessToken    string    // JWT token
	TokenExpiresAt time.Time // JWT expiration time
	PrivateKey     *rsa.PrivateKey
	KdfConfig      *models.KdfConfiguration

	// Organizations data
	Organizations map[string]OrganizationSecret
}

// validateCredentials ensures that the provided credentials meet the requirements
func (c *Client) validateCredentials() error {
	if c.Credentials == nil {
		return fmt.Errorf("no credentials provided")
	}

	// Check if at least one auth method is defined
	hasAdminAuth := c.Credentials.AdminToken != ""
	hasUserAuth := c.Credentials.Email != "" && c.Credentials.MasterPassword != ""
	hasOAuth2Auth := c.Credentials.ClientID != "" && c.Credentials.ClientSecret != ""

	if !hasAdminAuth && !hasUserAuth && !hasOAuth2Auth {
		return fmt.Errorf("at least one authentication method must be provided")
	}

	// Validate user credentials if OAuth2 is used
	if hasOAuth2Auth {
		if c.Credentials.Email == "" || c.Credentials.MasterPassword == "" {
			return fmt.Errorf("email and master password are required when using OAuth2")
		}
		c.userAuthMethod = AuthMethodOAuth2
	} else if hasUserAuth {
		c.userAuthMethod = AuthMethodUserPassword
	}

	return nil
}

// getAuthMethod determines which authentication method to use based on the request path
func (c *Client) getAuthMethod(path string) (AuthMethod, error) {
	// Use admin token for /admin endpoints
	if strings.HasPrefix(path, "/admin") {
		if c.Credentials.AdminToken != "" {
			return AuthMethodAdmin, nil
		}
		return AuthMethodNone, fmt.Errorf("admin token is required for admin endpoints but was not provided")
	}

	// For other endpoints, prefer OAuth2 if available
	if c.Credentials.ClientID != "" && c.Credentials.ClientSecret != "" {
		return AuthMethodOAuth2, nil
	}

	// Fall back to user/password if available
	if c.Credentials.Email != "" && c.Credentials.MasterPassword != "" {
		return AuthMethodUserPassword, nil
	}

	return AuthMethodNone, fmt.Errorf("no valid authentication method available for path: %s", path)
}

// authenticateRequest adds authentication headers/data to the request based on the auth method
func (c *Client) authenticateRequest(req *http.Request) error {
	authMethod, err := c.getAuthMethod(req.URL.Path)
	if err != nil {
		return err
	}

	switch authMethod {
	case AuthMethodAdmin:
		// Ensure we have valid admin authentication
		if err := c.ensureAdminAuth(req.Context()); err != nil {
			return fmt.Errorf("admin authentication failed: %w", err)
		}

		// Add admin cookie to request
		if c.AuthState.AdminCookie != nil {
			req.AddCookie(c.AuthState.AdminCookie)
		}
	case AuthMethodOAuth2, AuthMethodUserPassword:
		// Both OAuth2 and user/password methods use JWT tokens
		if err := c.ensureUserAuth(req.Context()); err != nil {
			return fmt.Errorf("user authentication failed: %w", err)
		}

		// Add the JWT token as a Bearer token
		if c.AuthState.AccessToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.AuthState.AccessToken)
		}
	case AuthMethodNone:
		return fmt.Errorf("no valid authentication method available for path: %s", req.URL.Path)
	}

	return nil
}

// Re
