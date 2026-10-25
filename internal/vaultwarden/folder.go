package vaultwarden

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// folderRequest is the request body for creating and renaming a folder.
type folderRequest struct {
	Name string `json:"name"`
}

// folderError maps the errors of the folder endpoints. Vaultwarden answers 400 "Invalid folder"
// for a folder that does not exist or belongs to another user. A 404 is treated the same.
func folderError(folderID string, err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound ||
		(apiErr.StatusCode == http.StatusBadRequest && strings.Contains(apiErr.Body, "Invalid folder"))) {
		return fmt.Errorf("folder %s: %w", folderID, ErrNotFound)
	}
	return err
}

// decryptFolder decrypts the name of a folder with the user key.
func (c *Client) decryptFolder(ctx context.Context, folder *models.Folder) error {
	key, err := c.UserKey(ctx)
	if err != nil {
		return err
	}
	name, err := decryptString(folder.Name, key)
	if err != nil {
		return fmt.Errorf("failed to decrypt the name of folder %s: %w", folder.ID, err)
	}
	folder.Name = name
	return nil
}

// encryptFolderName encrypts the name of a folder with the user key.
func (c *Client) encryptFolderName(ctx context.Context, name string) (string, error) {
	key, err := c.UserKey(ctx)
	if err != nil {
		return "", err
	}
	encrypted, err := encryptString(name, key)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt folder name: %w", err)
	}
	return encrypted, nil
}

// CreateFolder creates a folder in the personal vault of the provider user and returns it with
// its name decrypted.
func (c *Client) CreateFolder(ctx context.Context, name string) (*models.Folder, error) {
	encryptedName, err := c.encryptFolderName(ctx, name)
	if err != nil {
		return nil, err
	}

	var folder models.Folder
	if _, err := c.doRequest(ctx, http.MethodPost, "/api/folders", folderRequest{Name: encryptedName}, &folder); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	if err := c.decryptFolder(ctx, &folder); err != nil {
		return nil, err
	}
	return &folder, nil
}

// GetFolder returns a folder of the provider user with its name decrypted.
func (c *Client) GetFolder(ctx context.Context, folderID string) (*models.Folder, error) {
	var folder models.Folder
	if _, err := c.doRequest(ctx, http.MethodGet, "/api/folders/"+folderID, nil, &folder); err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", folderError(folderID, err))
	}

	if err := c.decryptFolder(ctx, &folder); err != nil {
		return nil, err
	}
	return &folder, nil
}

// ListFolders returns the folders of the provider user with their names decrypted.
func (c *Client) ListFolders(ctx context.Context) ([]models.Folder, error) {
	var folders models.Folders
	if _, err := c.doRequest(ctx, http.MethodGet, "/api/folders", nil, &folders); err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	for i := range folders.Data {
		if err := c.decryptFolder(ctx, &folders.Data[i]); err != nil {
			return nil, err
		}
	}
	return folders.Data, nil
}

// UpdateFolder renames a folder of the provider user.
func (c *Client) UpdateFolder(ctx context.Context, folderID, name string) (*models.Folder, error) {
	encryptedName, err := c.encryptFolderName(ctx, name)
	if err != nil {
		return nil, err
	}

	var folder models.Folder
	if _, err := c.doRequest(ctx, http.MethodPut, "/api/folders/"+folderID, folderRequest{Name: encryptedName}, &folder); err != nil {
		return nil, fmt.Errorf("failed to update folder: %w", folderError(folderID, err))
	}

	if err := c.decryptFolder(ctx, &folder); err != nil {
		return nil, err
	}
	return &folder, nil
}

// DeleteFolder deletes a folder of the provider user. The items in the folder are kept and
// moved out of it. Deleting a folder that does not exist succeeds.
func (c *Client) DeleteFolder(ctx context.Context, folderID string) error {
	if _, err := c.doRequest(ctx, http.MethodDelete, "/api/folders/"+folderID, nil, nil); err != nil {
		if err = folderError(folderID, err); IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete folder: %w", err)
	}
	return nil
}
