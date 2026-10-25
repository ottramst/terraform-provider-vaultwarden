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

// CreateOrganization creates a new Vaultwarden organization.
func (c *Client) CreateOrganization(ctx context.Context, org models.Organization) (*models.Organization, error) {
	// First ensure we have valid authentication and thus the private key
	accessToken, privateKey, err := c.userSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Create a shared key for the organization

	encSharedKey, sharedKey, err := keybuilder.GenerateSharedKey(&privateKey.PublicKey)
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
		return nil, fmt.Errorf("failed to generate organization key pair: %w", err)
	}

	// Add the keys to the organization
	org.Keys = models.KeyPair{
		PublicKey:           publicKey,
		EncryptedPrivateKey: encryptedPrivateKey,
	}

	// Set billing email to current user's email if not provided
	if org.BillingEmail == "" {
		email, err := helpers.ParseJWTEmail(accessToken)
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
	c.setOrganizationSecret(OrganizationSecret{
		Key:              *sharedKey,
		OrganizationUUID: orgResp.ID,
		Name:             orgResp.Name,
	})

	return &orgResp, nil
}

// GetOrganization retrieves an organization by its ID.
func (c *Client) GetOrganization(ctx context.Context, ID string) (*models.Organization, error) {
	if ID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	var org models.Organization
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s", ID), nil, &org); err != nil {
		if c.organizationGone(ctx, ID, err) {
			return nil, fmt.Errorf("organization %s: %w", ID, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	return &org, nil
}

// UpdateOrganization updates an organization by its ID.
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

// DeleteOrganizationRequest represents the request body for deleting an organization.
type DeleteOrganizationRequest struct {
	MasterPasswordHash string `json:"masterPasswordHash"`
}

// DeleteOrganization deletes an organization by its ID.
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
		// Already deleted
		if c.organizationGone(ctx, ID, err) {
			return nil
		}
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	return nil
}

// InviteOrganizationUserRequest represents the request body for inviting a user to an organization.
type InviteOrganizationUserRequest struct {
	Emails               []string           `json:"emails"`
	Collections          []string           `json:"collections"`
	AccessAll            bool               `json:"accessAll"`
	AccessSecretsManager bool               `json:"accessSecretsManager"`
	Type                 models.UserOrgType `json:"type"`
	Groups               []string           `json:"groups"`
	Permissions          map[string]bool    `json:"permissions,omitempty"`
}

// UpdateOrganizationUserRequest represents the request body for updating a user in an organization.
// Vaultwarden replaces the user's collection and group assignments with the ones sent.
type UpdateOrganizationUserRequest struct {
	Type        models.UserOrgType        `json:"type"`
	AccessAll   bool                      `json:"accessAll"`
	Permissions map[string]bool           `json:"permissions,omitempty"`
	Collections []models.CollectionAccess `json:"collections"`
	Groups      []string                  `json:"groups"`
}

// OrganizationUserRole returns the type and permissions to send for a role. Vaultwarden 1.34+
// ignores accessAll and only grants a manager access to all collections when it is sent as a
// custom role with the manage-all-collections permissions.
func OrganizationUserRole(userType models.UserOrgType, accessAll bool) (models.UserOrgType, map[string]bool) {
	if userType == models.UserOrgTypeManager && accessAll {
		return models.UserOrgTypeCustom, models.ManageAllCollectionsPermissions
	}

	return userType, nil
}

// InviteOrganizationUser invites a new user to an organization.
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

// GetOrganizationUsers retrieves all users in an organization.
func (c *Client) GetOrganizationUsers(ctx context.Context, orgID string) (*models.OrganizationUsers, error) {
	var users models.OrganizationUsers
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/users", orgID), nil, &users); err != nil {
		return nil, fmt.Errorf("failed to get organization users: %w", err)
	}

	return &users, nil
}

// GetOrganizationUserByEmail retrieves a user in an organization by their email.
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

// GetOrganizationUser retrieves a user in an organization by their ID, including the
// collections and groups assigned to them directly.
func (c *Client) GetOrganizationUser(ctx context.Context, userID, orgID string) (*models.OrganizationUserDetails, error) {
	var user models.OrganizationUserDetails
	path := fmt.Sprintf("/api/organizations/%s/users/%s?includeCollections=true&includeGroups=true", orgID, userID)
	if _, err := c.doRequest(ctx, http.MethodGet, path, nil, &user); err != nil {
		// Vaultwarden answers 400 for a member that is not part of the organization
		if hasStatus(err, http.StatusBadRequest) || c.organizationGone(ctx, orgID, err) {
			return nil, fmt.Errorf("organization user %s: %w", userID, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get organization user: %w", err)
	}

	return &user, nil
}

// DeleteOrganizationUser deletes a user in an organization by their ID.
func (c *Client) DeleteOrganizationUser(ctx context.Context, userID, orgID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/organizations/%s/users/%s", orgID, userID), nil, nil); err != nil {
		// Already removed
		if _, getErr := c.GetOrganizationUser(ctx, userID, orgID); IsNotFound(getErr) {
			return nil
		}
		return fmt.Errorf("failed to delete organization user: %w", err)
	}

	return nil
}

// organizationCollectionsAccess is the response of the organization collection details endpoint.
type organizationCollectionsAccess struct {
	Data []struct {
		ID    string                    `json:"id"`
		Users []models.CollectionAccess `json:"users"`
	} `json:"data"`
}

// getOrganizationUserCollectionAccess returns the collections a user is assigned to directly.
// The user details endpoint only lists them for confirmed users before Vaultwarden 1.37, so they
// are collected from the organization's collection details instead.
func (c *Client) getOrganizationUserCollectionAccess(ctx context.Context, orgID, userID string) ([]models.CollectionAccess, error) {
	var details organizationCollectionsAccess
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/collections/details", orgID), nil, &details); err != nil {
		return nil, fmt.Errorf("failed to get organization collection access: %w", err)
	}

	collections := []models.CollectionAccess{}
	for _, collection := range details.Data {
		for _, user := range collection.Users {
			if user.ID == userID {
				user.ID = collection.ID
				collections = append(collections, user)
			}
		}
	}

	return collections, nil
}

// UpdateOrganizationUser updates the type of a user in an organization by their ID. The user's
// existing collection and group assignments are preserved.
func (c *Client) UpdateOrganizationUser(ctx context.Context, userID, orgID string, req UpdateOrganizationUserRequest) error {
	defer c.lockOrganizationAccess(orgID)()

	current, err := c.GetOrganizationUser(ctx, userID, orgID)
	if err != nil {
		return err
	}

	req.Collections, err = c.getOrganizationUserCollectionAccess(ctx, orgID, userID)
	if err != nil {
		return err
	}
	req.Groups = current.Groups
	if req.Groups == nil {
		req.Groups = []string{}
	}

	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/organizations/%s/users/%s", orgID, userID), req, nil); err != nil {
		return fmt.Errorf("failed to update organization user: %w", err)
	}

	return nil
}
