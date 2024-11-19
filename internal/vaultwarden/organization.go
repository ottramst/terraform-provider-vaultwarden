package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/helpers"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
)

// CreateOrganization creates a new Vaultwarden organization
func (c *Client) CreateOrganization(ctx context.Context, org models.Organization) (*models.Organization, error) {
	// First ensure we have valid authentication and thus the private key
	if err := c.ensureUserAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Create a shared key for the organization
	encSharedKey, sharedKey, err := keybuilder.GenerateSharedKey(&c.AuthState.PrivateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate shared key: %w", err)
	}
	org.Key = encSharedKey

	// Encrypt the collection name
	collectionName, err := crypt.EncryptAsString([]byte(org.CollectionName), *sharedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt collection name: %w", err)
	}
	org.CollectionName = collectionName

	// Create the public and private keys
	publicKey, encryptedPrivateKey, err := keybuilder.GenerateEncryptedRSAKeyPair(*sharedKey)
	if err != nil {
		panic(err)
	}

	// Add the keys to the organization
	org.Keys = models.KeyPair{
		PublicKey:           publicKey,
		EncryptedPrivateKey: encryptedPrivateKey,
	}

	// Set billing email to current user's email if not provided
	if org.BillingEmail == "" {
		email, err := helpers.ParseJWTEmail(c.AuthState.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to get user email from token: %w", err)
		}
		org.BillingEmail = email
	}

	var orgResp models.Organization
	if _, err := c.doRequest(ctx, http.MethodPost, "/api/organizations", org, &orgResp); err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return &orgResp, nil
}

// GetOrganization retrieves an organization by its ID
func (c *Client) GetOrganization(ctx context.Context, ID string) (*models.Organization, error) {
	if ID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	var org models.Organization
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s", ID), nil, &org); err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	return &org, nil
}

// UpdateOrganization updates an organization by its ID
func (c *Client) UpdateOrganization(ctx context.Context, ID string, org models.Organization) (*models.Organization, error) {
	if ID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	var orgResp models.Organization
	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/organizations/%s", ID), org, &orgResp); err != nil {
		return nil, fmt.Errorf("failed to update organization: %w", err)
	}

	return &orgResp, nil
}

// DeleteOrganizationRequest represents the request body for deleting an organization
type DeleteOrganizationRequest struct {
	MasterPasswordHash string `json:"masterPasswordHash"`
}

// DeleteOrganization deletes an organization by its ID
func (c *Client) DeleteOrganization(ctx context.Context, ID string) error {
	if ID == "" {
		return fmt.Errorf("organization ID is required")
	}

	// Do a prelogin to fetch KDF parameters
	preloginResp, err := c.PreLogin(ctx)
	if err != nil {
		return fmt.Errorf("prelogin failed: %w", err)
	}

	// Create KDF configuration
	kdfConfig := &models.KdfConfiguration{
		KdfType:        preloginResp.Kdf,
		KdfIterations:  preloginResp.KdfIterations,
		KdfMemory:      preloginResp.KdfMemory,
		KdfParallelism: preloginResp.KdfParallelism,
	}

	preloginKey, err := keybuilder.BuildPreloginKey(c.Credentials.MasterPassword, c.Credentials.Email, kdfConfig)
	if err != nil {
		return fmt.Errorf("failed to build prelogin key: %w", err)
	}

	// Step 3: Hash password
	hashedPassword := crypt.HashPassword(c.Credentials.MasterPassword, *preloginKey, false)

	body := DeleteOrganizationRequest{
		MasterPasswordHash: hashedPassword,
	}

	if _, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/organizations/%s", ID), body, nil); err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	return nil
}
