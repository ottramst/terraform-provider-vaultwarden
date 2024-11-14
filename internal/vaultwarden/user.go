package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
	"net/mail"
)

// InviteUser invites a new user to Vaultwarden
func (c *Client) InviteUser(ctx context.Context, user models.User) (*models.User, error) {
	// Validate email format
	if _, err := mail.ParseAddress(user.Email); err != nil {
		return nil, fmt.Errorf("invalid email format: %s", user.Email)
	}

	var userResp models.User
	if err := c.Post(ctx, "/admin/invite", user, &userResp); err != nil {
		return nil, fmt.Errorf("failed to invite user: %w", err)
	}

	return &userResp, nil
}

// GetUser retrieves a user by their ID
func (c *Client) GetUser(ctx context.Context, ID string) (*models.User, error) {
	if ID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	var user models.User
	if err := c.Get(ctx, fmt.Sprintf("/admin/users/%s", ID), &user); err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// DeleteUser deletes a user by their ID
func (c *Client) DeleteUser(ctx context.Context, ID string) error {
	if ID == "" {
		return fmt.Errorf("user ID is required")
	}

	if err := c.Post(ctx, fmt.Sprintf("/admin/users/%s/delete", ID), nil, nil); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// RegisterUserRequest represents the request body for registering a user
type RegisterUserRequest struct {
	Email              string         `json:"email"`
	MasterPasswordHash string         `json:"masterPasswordHash"`
	Key                string         `json:"key"`
	Kdf                models.KdfType `json:"kdf"`
	KdfIterations      int            `json:"kdfIterations"`
	KdfMemory          int            `json:"kdfMemory"`
	KdfParallelism     int            `json:"kdfParallelism"`
	Keys               models.KeyPair `json:"keys"`
}

// RegisterUser registers a new user to Vaultwarden
func (c *Client) RegisterUser(ctx context.Context, req RegisterUserRequest) (*models.User, error) {
	var user models.User
	if err := c.doUnauthenticatedRequest(ctx, http.MethodPost, "/api/accounts/register", req, &user); err != nil {
		return nil, fmt.Errorf("failed to register user: %w", err)
	}

	return &user, nil
}
