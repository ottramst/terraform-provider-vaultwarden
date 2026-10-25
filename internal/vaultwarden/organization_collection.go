package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/encryptedstring"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
)

// CreateOrganizationCollection creates a new Vaultwarden organization collection.
func (c *Client) CreateOrganizationCollection(ctx context.Context, orgID string, collection models.Collection) (*models.Collection, error) {
	// First ensure we have valid authentication
	if err := c.ensureUserAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Get organization data from cache
	orgSecret, exists := c.OrganizationSecret(orgID)
	if !exists {
		return nil, fmt.Errorf("organization %s not found in cache", orgID)
	}

	// Encrypt the collection name using the cached key
	collectionName, err := crypt.EncryptAsString([]byte(collection.Name), orgSecret.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt collection name: %w", err)
	}
	collection.Name = collectionName

	// Set empty lists for groups and users when none are provided
	if collection.Groups == nil {
		collection.Groups = []models.CollectionAccess{}
	}

	if collection.Users == nil {
		collection.Users = []models.CollectionAccess{}
	}

	var collectionResp models.Collection
	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/organizations/%s/collections", orgID), collection, &collectionResp); err != nil {
		return nil, fmt.Errorf("failed to create organization collection: %w", err)
	}

	return &collectionResp, nil
}

// GetOrganizationCollection retrieves a specific collection from an organization.
func (c *Client) GetOrganizationCollection(ctx context.Context, orgID string, collectionID string) (*models.Collection, error) {
	var listResp models.OrganizationCollections
	if _, err := c.doRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/api/organizations/%s/collections", orgID),
		nil,
		&listResp,
	); err != nil {
		if c.organizationGone(ctx, orgID, err) {
			return nil, fmt.Errorf("organization %s: %w", orgID, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to list organization collections: %w", err)
	}

	// Find the specific collection in the response
	for _, collection := range listResp.Data {
		if collection.ID == collectionID {
			return &collection, nil
		}
	}

	return nil, fmt.Errorf("collection %s in organization %s: %w", collectionID, orgID, ErrNotFound)
}

// ListOrganizationCollections returns the collections of an organization with their names decrypted.
func (c *Client) ListOrganizationCollections(ctx context.Context, orgID string) ([]models.Collection, error) {
	var listResp models.OrganizationCollections
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/collections", orgID), nil, &listResp); err != nil {
		if c.organizationGone(ctx, orgID, err) {
			return nil, fmt.Errorf("organization %s: %w", orgID, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to list organization collections: %w", err)
	}

	orgSecret, exists := c.OrganizationSecret(orgID)
	if !exists {
		return nil, fmt.Errorf("the key of organization %s is not available: the provider user is not a confirmed member of it", orgID)
	}

	collections := make([]models.Collection, 0, len(listResp.Data))
	for _, collection := range listResp.Data {
		encryptedName, err := encryptedstring.NewFromEncryptedValue(collection.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to parse the name of collection %s: %w", collection.ID, err)
		}
		name, err := crypt.Decrypt(encryptedName, &orgSecret.Key)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt the name of collection %s: %w", collection.ID, err)
		}
		collection.Name = string(name)
		collections = append(collections, collection)
	}

	return collections, nil
}

// GetOrganizationCollectionAccess returns the users and groups assigned to a collection.
func (c *Client) GetOrganizationCollectionAccess(ctx context.Context, orgID, colID string) (*models.CollectionAccessDetails, error) {
	var details models.CollectionAccessDetails
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/organizations/%s/collections/%s/details", orgID, colID), nil, &details); err != nil {
		return nil, fmt.Errorf("failed to get organization collection access: %w", err)
	}

	return &details, nil
}

// bulkCollectionAccessRequest is the request body for setting the access to collections.
type bulkCollectionAccessRequest struct {
	CollectionIDs []string                  `json:"collectionIds"`
	Users         []models.CollectionAccess `json:"users"`
	Groups        []models.CollectionAccess `json:"groups"`
}

// SetOrganizationCollectionAccess replaces the users and groups assigned to a collection.
func (c *Client) SetOrganizationCollectionAccess(ctx context.Context, orgID, colID string, access models.CollectionAccessDetails) error {
	req := bulkCollectionAccessRequest{
		CollectionIDs: []string{colID},
		Users:         access.Users,
		Groups:        access.Groups,
	}
	if req.Users == nil {
		req.Users = []models.CollectionAccess{}
	}
	if req.Groups == nil {
		req.Groups = []models.CollectionAccess{}
	}

	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/organizations/%s/collections/bulk-access", orgID), req, nil); err != nil {
		return fmt.Errorf("failed to set organization collection access: %w", err)
	}

	return nil
}

// UpdateOrganizationCollection updates an existing Vaultwarden organization collection.
func (c *Client) UpdateOrganizationCollection(ctx context.Context, orgID, colID string, collection models.Collection) (*models.Collection, error) {
	// First ensure we have valid authentication
	if err := c.ensureUserAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Get organization data from cache
	orgSecret, exists := c.OrganizationSecret(orgID)
	if !exists {
		return nil, fmt.Errorf("organization %s not found in cache", orgID)
	}

	// Encrypt the collection name using the cached key
	collectionName, err := crypt.EncryptAsString([]byte(collection.Name), orgSecret.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt collection name: %w", err)
	}
	collection.Name = collectionName

	// Vaultwarden replaces the collection's user and group assignments with the ones
	// sent, so send back the current ones
	defer c.lockOrganizationAccess(orgID)()
	access, err := c.GetOrganizationCollectionAccess(ctx, orgID, colID)
	if err != nil {
		return nil, err
	}
	collection.Users = access.Users
	if collection.Users == nil {
		collection.Users = []models.CollectionAccess{}
	}
	collection.Groups = access.Groups
	if collection.Groups == nil {
		collection.Groups = []models.CollectionAccess{}
	}

	var collectionResp models.Collection
	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/organizations/%s/collections/%s", orgID, colID), collection, &collectionResp); err != nil {
		return nil, fmt.Errorf("failed to update organization collection: %w", err)
	}

	return &collectionResp, nil
}

// GetOrganizationCollectionUser returns the access of an organization user to a collection.
func (c *Client) GetOrganizationCollectionUser(ctx context.Context, orgID, colID, userID string) (*models.CollectionAccess, error) {
	access, err := c.GetOrganizationCollectionAccess(ctx, orgID, colID)
	if err != nil {
		if _, getErr := c.GetOrganizationCollection(ctx, orgID, colID); IsNotFound(getErr) {
			return nil, getErr
		}
		return nil, err
	}

	for _, user := range access.Users {
		if user.ID == userID {
			return &user, nil
		}
	}

	return nil, fmt.Errorf("organization user %s in collection %s: %w", userID, colID, ErrNotFound)
}

// SetOrganizationCollectionUser grants an organization user access to a collection, or changes
// it. The access of other users and groups is preserved.
func (c *Client) SetOrganizationCollectionUser(ctx context.Context, orgID, colID string, userAccess models.CollectionAccess) error {
	defer c.lockOrganizationAccess(orgID)()

	access, err := c.GetOrganizationCollectionAccess(ctx, orgID, colID)
	if err != nil {
		return err
	}

	users := []models.CollectionAccess{userAccess}
	for _, user := range access.Users {
		if user.ID != userAccess.ID {
			users = append(users, user)
		}
	}
	access.Users = users

	return c.SetOrganizationCollectionAccess(ctx, orgID, colID, *access)
}

// RemoveOrganizationCollectionUser removes the access of an organization user to a collection.
// The access of other users and groups is preserved.
func (c *Client) RemoveOrganizationCollectionUser(ctx context.Context, orgID, colID, userID string) error {
	defer c.lockOrganizationAccess(orgID)()

	access, err := c.GetOrganizationCollectionAccess(ctx, orgID, colID)
	if err != nil {
		// Collection already deleted
		if _, getErr := c.GetOrganizationCollection(ctx, orgID, colID); IsNotFound(getErr) {
			return nil
		}
		return err
	}

	users := []models.CollectionAccess{}
	for _, user := range access.Users {
		if user.ID != userID {
			users = append(users, user)
		}
	}
	if len(users) == len(access.Users) {
		return nil
	}
	access.Users = users

	return c.SetOrganizationCollectionAccess(ctx, orgID, colID, *access)
}

// DeleteOrganizationCollection deletes a collection from an organization.
func (c *Client) DeleteOrganizationCollection(ctx context.Context, orgID, colID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/organizations/%s/collections/%s", orgID, colID), nil, nil); err != nil {
		// Already deleted
		if _, getErr := c.GetOrganizationCollection(ctx, orgID, colID); IsNotFound(getErr) {
			return nil
		}
		return fmt.Errorf("failed to delete organization collection: %w", err)
	}

	return nil
}
