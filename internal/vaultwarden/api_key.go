package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
)

// APIKey is the client ID and secret of an organization's or user's API key.
type APIKey struct {
	ClientID     string
	ClientSecret string
	// RevisionDate is when the key was created or last rotated
	RevisionDate string
}

// apiKeyResponse represents the response of the api-key and rotate-api-key endpoints.
type apiKeyResponse struct {
	APIKey       string `json:"apiKey"`
	RevisionDate string `json:"revisionDate"`
}

// passwordVerificationRequest represents the request body of endpoints that require the master
// password hash of the user, such as deleting an organization or reading an API key.
type passwordVerificationRequest struct {
	MasterPasswordHash string `json:"masterPasswordHash"`
}

// MasterPasswordHash returns the server-side hash of the provider user's master password, which
// Vaultwarden requires to confirm sensitive actions.
func (c *Client) MasterPasswordHash(ctx context.Context) (string, error) {
	preloginResp, err := c.PreLogin(ctx)
	if err != nil {
		return "", fmt.Errorf("prelogin failed: %w", err)
	}

	kdfConfig := &models.KdfConfiguration{
		KdfType:        preloginResp.Kdf,
		KdfIterations:  preloginResp.KdfIterations,
		KdfMemory:      preloginResp.KdfMemory,
		KdfParallelism: preloginResp.KdfParallelism,
	}

	preloginKey, err := keybuilder.BuildPreloginKey(c.Credentials.MasterPassword, c.Credentials.Email, kdfConfig)
	if err != nil {
		return "", fmt.Errorf("failed to build prelogin key: %w", err)
	}

	return crypt.HashPassword(c.Credentials.MasterPassword, *preloginKey, false), nil
}

// passwordVerification returns the request body that confirms an action with the master password.
func (c *Client) passwordVerification(ctx context.Context) (passwordVerificationRequest, error) {
	hash, err := c.MasterPasswordHash(ctx)
	if err != nil {
		return passwordVerificationRequest{}, err
	}
	return passwordVerificationRequest{MasterPasswordHash: hash}, nil
}

// GetOrganizationAPIKey returns the API key of an organization, creating it if the organization
// has none yet. With rotate, a new secret replaces the current one. Vaultwarden 1.37 and later
// only allow owners to read or rotate it; 1.36 also allows admins.
func (c *Client) GetOrganizationAPIKey(ctx context.Context, orgID string, rotate bool) (*APIKey, error) {
	if orgID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	body, err := c.passwordVerification(ctx)
	if err != nil {
		return nil, err
	}

	endpoint := "api-key"
	if rotate {
		endpoint = "rotate-api-key"
	}

	var resp apiKeyResponse
	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/organizations/%s/%s", orgID, endpoint), body, &resp); err != nil {
		if c.organizationGone(ctx, orgID, err) {
			return nil, fmt.Errorf("organization %s: %w", orgID, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get organization API key: %w", err)
	}

	return &APIKey{
		ClientID:     "organization." + orgID,
		ClientSecret: resp.APIKey,
		RevisionDate: resp.RevisionDate,
	}, nil
}

// GetUserAPIKey returns the personal API key of the provider user, creating it if the user has
// none yet.
func (c *Client) GetUserAPIKey(ctx context.Context) (*APIKey, error) {
	profile, err := c.GetProfile(ctx)
	if err != nil {
		return nil, err
	}

	body, err := c.passwordVerification(ctx)
	if err != nil {
		return nil, err
	}

	var resp apiKeyResponse
	if _, err := c.doRequest(ctx, http.MethodPost, "/api/accounts/api-key", body, &resp); err != nil {
		return nil, fmt.Errorf("failed to get user API key: %w", err)
	}

	return &APIKey{
		ClientID:     "user." + profile.ID,
		ClientSecret: resp.APIKey,
		RevisionDate: resp.RevisionDate,
	}, nil
}
