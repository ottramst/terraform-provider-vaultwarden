package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/helpers"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// CreateOrganization creates a new Vaultwarden organization
func (c *Client) CreateOrganization(ctx context.Context, org models.Organization) (*models.Organization, error) {
	// Ensure we have valid authentication and thus the private key
	if err := c.ensureValidAuth(ctx, "/api/organizations"); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Verify we have the required private key
	if c.privateKey == nil {
		return nil, fmt.Errorf("private key not available, authentication may have failed")
	}

	// Create a shared key for the organization
	encSharedKey, sharedKey, err := keybuilder.GenerateSharedKey(&c.privateKey.PublicKey)
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
	publicKey, encryptedPrivateKey, err := keybuilder.GenerateRSAKeyPair(*sharedKey)
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
		claims, err := helpers.ParseJWT(c.accessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to get user email from token: %w", err)
		}
		org.BillingEmail = claims.Email
	}

	var orgResp models.Organization
	if err := c.Post(ctx, "/api/organizations", org, &orgResp); err != nil {
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
	if err := c.Get(ctx, fmt.Sprintf("/api/organizations/%s", ID), &org); err != nil {
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
	if err := c.Put(ctx, fmt.Sprintf("/api/organizations/%s", ID), org, &orgResp); err != nil {
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
	preloginResp, err := c.Prelogin(ctx)
	if err != nil {
		return fmt.Errorf("prelogin failed: %w", err)
	}

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

	body := DeleteOrganizationRequest{
		MasterPasswordHash: hashedPassword,
	}

	if err := c.Delete(ctx, fmt.Sprintf("/api/organizations/%s", ID), body); err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	return nil
}
