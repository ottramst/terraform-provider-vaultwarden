package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/crypt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
)

// CreateOrganizationCollection creates a new Vaultwarden organization collection
func (c *Client) CreateOrganizationCollection(ctx context.Context, orgID string, collection models.Collection) (*models.Collection, error) {
	// First ensure we have valid authentication
	if err := c.ensureUserAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Get organization data from cache
	orgSecret, exists := c.AuthState.Organizations[orgID]
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
		collection.Groups = []string{}
	}

	if collection.Users == nil {
		collection.Users = []string{}
	}

	var collectionResp models.Collection
	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/api/organizations/%s/collections", orgID), collection, &collectionResp); err != nil {
		return nil, fmt.Errorf("failed to create organization collection: %w", err)
	}

	return &collectionResp, nil
}

// GetOrganizationCollection retrieves a specific collection from an organization
func (c *Client) GetOrganizationCollection(ctx context.Context, orgID string, collectionID string) (*models.Collection, error) {
	var listResp models.OrganizationCollections
	if _, err := c.doRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("/api/organizations/%s/collections", orgID),
		nil,
		&listResp,
	); err != nil {
		return nil, fmt.Errorf("failed to list organization collections: %w", err)
	}

	// Find the specific collection in the response
	for _, collection := range listResp.Data {
		if collection.ID == collectionID {
			return &collection, nil
		}
	}

	return nil, fmt.Errorf("collection %s not found in organization %s", collectionID, orgID)
}

// UpdateOrganizationCollection updates an existing Vaultwarden organization collection
func (c *Client) UpdateOrganizationCollection(ctx context.Context, orgID, colID string, collection models.Collection) (*models.Collection, error) {
	// First ensure we have valid authentication
	if err := c.ensureUserAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	// Get organization data from cache
	orgSecret, exists := c.AuthState.Organizations[orgID]
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
		collection.Groups = []string{}
	}

	if collection.Users == nil {
		collection.Users = []string{}
	}

	var collectionResp models.Collection
	if _, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/api/organizations/%s/collections/%s", orgID, colID), collection, &collectionResp); err != nil {
		return nil, fmt.Errorf("failed to update organization collection: %w", err)
	}

	return &collectionResp, nil
}

func (c *Client) DeleteOrganizationCollection(ctx context.Context, orgID, colID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/organizations/%s/collections/%s", orgID, colID), nil, nil); err != nil {
		return fmt.Errorf("failed to delete organization collection: %w", err)
	}

	return nil
}
