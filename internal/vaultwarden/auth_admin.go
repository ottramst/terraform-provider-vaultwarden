package vaultwarden

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// adminCookie returns the cookie of a valid admin session, logging in first if needed.
// Concurrent callers wait for a single login.
func (c *Client) adminCookie(ctx context.Context) (*http.Cookie, error) {
	c.authMu.Lock()
	defer c.authMu.Unlock()

	// Check if we have a valid admin session
	if c.AuthState != nil && c.AuthState.AdminCookie != nil {
		// Check if cookie is not expired
		if !c.AuthState.AdminCookie.Expires.IsZero() && time.Now().Before(c.AuthState.AdminCookie.Expires) {
			return c.AuthState.AdminCookie, nil
		}
	}

	// Perform admin login
	if err := c.adminLogin(ctx); err != nil {
		return nil, err
	}

	return c.AuthState.AdminCookie, nil
}

// adminLogin performs the admin authentication. The caller must hold authMu.
func (c *Client) adminLogin(ctx context.Context) error {
	// Create form data
	form := url.Values{}
	form.Add("token", c.Credentials.AdminToken)

	// Make login request. Don't follow redirects, which some versions of Vaultwarden send.
	// See: https://github.com/dani-garcia/vaultwarden/issues/2444
	resp, err := c.send(ctx, http.MethodPost, "/admin", form, nil, requestOptions{noRedirects: true})
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
