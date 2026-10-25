package vaultwarden

import (
	"context"
	"errors"
	"fmt"
	"github.com/ottramst/terraform-provider-vaultwarden/internal/vaultwarden/models"
	"net/http"
	"net/mail"
	"strings"
)

// RegisterUserRequest represents the request body for registering a user.
type RegisterUserRequest struct {
	Email              string         `json:"email"`
	MasterPasswordHash string         `json:"masterPasswordHash"`
	Name               string         `json:"name"`
	Key                string         `json:"key"`
	Kdf                models.KdfType `json:"kdf"`
	KdfIterations      int            `json:"kdfIterations"`
	KdfMemory          int            `json:"kdfMemory"`
	KdfParallelism     int            `json:"kdfParallelism"`
	Keys               models.KeyPair `json:"keys"`
}

// registerVerificationRequest is the request body for starting a registration.
type registerVerificationRequest struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// registerFinishRequest is the request body for completing a registration.
type registerFinishRequest struct {
	RegisterUserRequest
	EmailVerificationToken string `json:"emailVerificationToken"`
}

// RegisterUser registers a new user.
//
// Registration is a two-step flow: the server first issues an email verification
// token, which is then used to finish the registration. The legacy single-step
// /api/accounts/register endpoint was removed in Vaultwarden 1.36.0.
func (c *Client) RegisterUser(ctx context.Context, req RegisterUserRequest) error {
	var token string
	verifyReq := registerVerificationRequest{Email: req.Email, Name: req.Name}
	if _, err := c.doUnauthenticatedRequest(ctx, http.MethodPost, "/identity/accounts/register/send-verification-email", verifyReq, &token); err != nil {
		return fmt.Errorf("failed to register user: %w", err)
	}

	// With SMTP configured and SIGNUPS_VERIFY enabled, the token is emailed to the
	// user instead of being returned, so the registration cannot be completed here.
	if token == "" {
		return fmt.Errorf("failed to register user: the server requires email verification for new accounts (SIGNUPS_VERIFY), which this provider cannot complete")
	}

	finishReq := registerFinishRequest{RegisterUserRequest: req, EmailVerificationToken: token}
	if _, err := c.doUnauthenticatedRequest(ctx, http.MethodPost, "/identity/accounts/register/finish", finishReq, nil); err != nil {
		return fmt.Errorf("failed to register user: %w", err)
	}

	return nil
}

// InviteUser invites a new user to Vaultwarden.
func (c *Client) InviteUser(ctx context.Context, user models.User) (*models.User, error) {
	// Validate email format
	if _, err := mail.ParseAddress(user.Email); err != nil {
		return nil, fmt.Errorf("invalid email format: %s", user.Email)
	}

	var userResp models.User
	if _, err := c.doRequest(ctx, http.MethodPost, "/admin/invite", user, &userResp); err != nil {
		return nil, fmt.Errorf("failed to invite user: %w", err)
	}

	return &userResp, nil
}

// GetUser retrieves a user by their ID.
func (c *Client) GetUser(ctx context.Context, ID string) (*models.User, error) {
	if ID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	var user models.User
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/admin/users/%s", ID), nil, &user); err != nil {
		if isUserNotFound(err) {
			return nil, fmt.Errorf("user %s: %w", ID, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// DeleteUser deletes a user by their ID.
func (c *Client) DeleteUser(ctx context.Context, ID string) error {
	if ID == "" {
		return fmt.Errorf("user ID is required")
	}

	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/admin/users/%s/delete", ID), struct{}{}, nil); err != nil {
		// Already deleted
		if isUserNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete user: %w", err)
	}

	return nil
}

// SetUserEnabled enables or disables a user. Disabling a user also logs them out of all their
// devices. The admin POST routes of Vaultwarden 1.37+ require a JSON body.
func (c *Client) SetUserEnabled(ctx context.Context, ID string, enabled bool) error {
	if ID == "" {
		return fmt.Errorf("user ID is required")
	}

	action := "disable"
	if enabled {
		action = "enable"
	}

	if _, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/admin/users/%s/%s", ID, action), struct{}{}, nil); err != nil {
		if isUserNotFound(err) {
			return fmt.Errorf("user %s: %w", ID, ErrNotFound)
		}
		return fmt.Errorf("failed to %s user: %w", action, err)
	}

	return nil
}

// isUserNotFound reports whether err is the admin API's answer for a user that does not exist.
// An unknown route is also a 404, but with an HTML page instead of a JSON error.
func isUserNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && strings.Contains(apiErr.Body, "User doesn't exist")
}

// GetUserByEmail retrieves a user by their email address.
func (c *Client) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	if _, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/admin/users/by-mail/%s", email), nil, &user); err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}
