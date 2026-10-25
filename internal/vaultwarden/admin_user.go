package vaultwarden

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
)

// GetAdminUser returns a user from the admin API by their ID.
func (c *Client) GetAdminUser(ctx context.Context, id string) (*models.AdminUser, error) {
	if id == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	var user models.AdminUser
	if _, err := c.doRequest(ctx, http.MethodGet, "/admin/users/"+url.PathEscape(id), nil, &user); err != nil {
		if isUserNotFound(err) {
			return nil, fmt.Errorf("user %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// GetAdminUserByEmail returns a user from the admin API by their email. Vaultwarden compares
// emails case-insensitively.
func (c *Client) GetAdminUserByEmail(ctx context.Context, email string) (*models.AdminUser, error) {
	if email == "" {
		return nil, fmt.Errorf("user email is required")
	}

	var user models.AdminUser
	if _, err := c.doRequest(ctx, http.MethodGet, "/admin/users/by-mail/"+url.PathEscape(email), nil, &user); err != nil {
		if isUserNotFound(err) {
			return nil, fmt.Errorf("user %s: %w", email, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// ListAdminUsers returns all users of the server from the admin API, including invited ones.
func (c *Client) ListAdminUsers(ctx context.Context) ([]models.AdminUser, error) {
	var users []models.AdminUser
	if _, err := c.doRequest(ctx, http.MethodGet, "/admin/users", nil, &users); err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	return users, nil
}
