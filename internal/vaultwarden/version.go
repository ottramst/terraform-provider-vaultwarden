package vaultwarden

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// MinimumSupportedVersion is the oldest Vaultwarden release line the provider supports and tests.
// Older versions get best-effort support only.
const MinimumSupportedVersion = "1.36.0"

// GetServerVersion returns the Vaultwarden server version, such as "1.37.4".
func (c *Client) GetServerVersion(ctx context.Context) (string, error) {
	var version string
	if _, err := c.doUnauthenticatedRequest(ctx, http.MethodGet, "/api/version", nil, &version); err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	return version, nil
}

// IsSupportedVersion reports whether version is at least MinimumSupportedVersion.
func IsSupportedVersion(version string) (bool, error) {
	got, err := parseVersion(version)
	if err != nil {
		return false, err
	}
	minimum, err := parseVersion(MinimumSupportedVersion)
	if err != nil {
		return false, err
	}

	for i := range got {
		if got[i] != minimum[i] {
			return got[i] > minimum[i], nil
		}
	}
	return true, nil
}

// parseVersion parses the major, minor and patch numbers of a version such as "1.37.4"
// or "1.37.4-abc123".
func parseVersion(version string) ([3]int, error) {
	var parsed [3]int

	core, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return parsed, fmt.Errorf("invalid version: %q", version)
	}

	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return parsed, fmt.Errorf("invalid version: %q", version)
		}
		parsed[i] = n
	}

	return parsed, nil
}
