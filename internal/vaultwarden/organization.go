package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/helpers"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/keybuilder"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
	"net/mail"
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

	// Cache the organization secret
	if c.AuthState != nil {
		if c.AuthState.Organizations == nil {
			c.AuthState.Organizations = make(map[string]OrganizationSecret)
		}
		c.AuthState.Organizations[orgResp.ID] = OrganizationSecret{
			Key:              *sharedKey,
			OrganizationUUID: orgResp.ID,
			Name:             orgResp.Name,
		}
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

// InviteOrganizationUserRequest represents the request body for inviting a user to an organization
type InviteOrganizationUserRequest struct {
	Emails               []string           `json:"emails"`
	Collections          []string           `json:"collections"`
	AccessAll            bool               `json:"accessAll"`
	AccessSecretsManager bool               `json:"accessSecretsManager"`
	Type                 models.UserOrgType `json:"type"`
	Groups               []string           `json:"groups"`
}

// InviteOrganizationUser invites a new user to an organization
func (c *Client) InviteOrganizationUser(ctx context.Context, req InviteOrganizationUserRequest, email, orgID string) error {
	// Validate email format
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("invalid email format: %s", email)
	}

	// Add the email to the request
	req.Emails = append(req.Emails, email)

	// Set an empty list for groups when none are provided
	if req.Groups == nil {
		req.Groups = []string{}
	}

	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/organizations/%s/users/invite", orgID), req, nil); err != nil {
		return fmt.Errorf("failed to invite user to organization: %w", err)
	}

	return nil
}

// GetOrganizationUsers retrieves all users in an organization
func (c *Client) GetOrganizationUsers(ctx context.Context, orgID string) (*models.OrganizationUsers, error) {
	var users models.OrganizationUsers
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/users", orgID), nil, &users); err != nil {
		return nil, fmt.Errorf("failed to get organization users: %w", err)
	}

	return &users, nil
}

// GetOrganizationUserByEmail retrieves a user in an organization by their email
func (c *Client) GetOrganizationUserByEmail(ctx context.Context, email, orgID string) (*models.OrganizationUserDetails, error) {
	// First get all users in the organization
	users, err := c.GetOrganizationUsers(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization users: %w", err)
	}

	// Find the user by email
	for _, user := range users.Data {
		if user.Email == email {
			return &user, nil
		}
	}

	return nil, fmt.Errorf("user not found in organization")
}

// GetOrganizationUser retrieves a user in an organization by their ID
func (c *Client) GetOrganizationUser(ctx context.Context, userID, orgID string) (*models.OrganizationUserDetails, error) {
	var user models.OrganizationUserDetails
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/users/%s", orgID, userID), nil, &user); err != nil {
		return nil, fmt.Errorf("failed to get organization user: %w", err)
	}

	return &user, nil
}

// DeleteOrganizationUser deletes a user in an organization by their ID
func (c *Client) DeleteOrganizationUser(ctx context.Context, userID, orgID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/organizations/%s/users/%s", orgID, userID), nil, nil); err != nil {
		return fmt.Errorf("failed to delete organization user: %w", err)
	}

	return nil
}
