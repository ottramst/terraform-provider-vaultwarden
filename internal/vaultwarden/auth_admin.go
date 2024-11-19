package vaultwarden

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ensureAdminAuth ensures that admin authentication is valid
func (c *Client) ensureAdminAuth(ctx context.Context) error {
	// Check if we have a valid admin session
	if c.AuthState != nil && c.AuthState.AdminCookie != nil {
		// Check if cookie is not expired
		if !c.AuthState.AdminCookie.Expires.IsZero() && time.Now().Before(c.AuthState.AdminCookie.Expires) {
			return nil
		}
	}

	// Perform admin login
	return c.adminLogin(ctx)
}

// adminLogin performs the admin authentication
func (c *Client) adminLogin(ctx context.Context) error {
	// Create form data
	form := url.Values{}
	form.Add("token", c.Credentials.AdminToken)

	// Configure client to not follow redirects. This happens for some versions of Vaultwarden
	// See: https://github.com/dani-garcia/vaultwarden/issues/2444
	originalClient := c.httpClient
	c.httpClient = &http.Client{
		Timeout: originalClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	defer func() {
		c.httpClient = originalClient
	}()

	// Make login request
	resp, err := c.doUnauthenticatedRequest(ctx, http.MethodPost, "/admin", form, nil)
	if err != nil {
		// Check if this is a redirect error (which we expect)
		if resp != nil && resp.StatusCode == http.StatusSeeOther {
			// This is fine, continue processing
		} else {
			return fmt.Errorf("admin login request failed: %w", err)
		}
	}
	defer resp.Body.Close()

	// Check for success (200 OK or 303 See Other)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusSeeOther {
		return fmt.Errorf("admin login failed with status code: %d", resp.StatusCode)
	}

	// Look for admin cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "VW_ADMIN" {
			if c.AuthState == nil {
				c.AuthState = &AuthState{}
			}
			c.AuthState.AdminCookie = cookie
			return nil
		}
	}

	return fmt.Errorf("admin login succeeded but no VM_ADMIN cookie was received")
}
