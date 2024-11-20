package vaultwarden

import (
	"context"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
)

// GetProfile retrieves the user's profile
func (c *Client) GetProfile(ctx context.Context) (*models.User, error) {
	// Ensure we have valid authentication
	if err := c.ensureUserAuth(ctx); err != nil {
		return nil, fmt.Errorf("authentication required: %w", err)
	}

	var user models.User
	if _, err := c.doRequest(ctx, http.MethodGet, "/api/accounts/profile", nil, &user); err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	return &user, nil
}
