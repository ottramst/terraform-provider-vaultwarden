package vaultwarden

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrNotFound is returned, wrapped, when the requested object does not exist on the server.
var ErrNotFound = errors.New("not found")

// IsNotFound reports whether err means the requested object does not exist.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// APIError is returned for responses with a non-2xx status code.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("request failed with status %d: %s. Response: %s", e.StatusCode, e.Status, e.Body)
}

// hasStatus reports whether err is an APIError with the given status code.
func hasStatus(err error, statusCode int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == statusCode
}

// organizationGone reports whether err from an organization-scoped request means the
// organization no longer exists. Vaultwarden answers requests for an organization the user
// is not a member of with a generic 401, which looks the same as an invalid session, so the
// user's profile is checked to tell the two apart.
func (c *Client) organizationGone(ctx context.Context, orgID string, err error) bool {
	if !hasStatus(err, http.StatusUnauthorized) {
		return false
	}

	profile, profileErr := c.GetProfile(ctx)
	if profileErr != nil {
		return false
	}

	for _, org := range profile.Organizations {
		if org.ID == orgID {
			return false
		}
	}

	return true
}
