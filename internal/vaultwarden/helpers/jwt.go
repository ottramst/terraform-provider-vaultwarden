package helpers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// JWTClaims represents the claims in the Vaultwarden JWT
type JWTClaims struct {
	NotBefore     int64    `json:"nbf"`
	ExpiresAt     int64    `json:"exp"`
	Issuer        string   `json:"iss"`
	Subject       string   `json:"sub"`
	Premium       bool     `json:"premium"`
	Name          string   `json:"name"`
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	SecurityStamp string   `json:"sstamp"`
	DeviceId      string   `json:"device"`
	Scope         []string `json:"scope"`
	AuthMethods   []string `json:"amr"`
}

// ParseJWT parses a JWT token string and returns the claims
func ParseJWT(tokenString string) (*JWTClaims, error) {
	// Split the token
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode the claims part (second part)
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode claims: %w", err)
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	return &claims, nil
}
