package models

// Credentials represents different types of authentication credentials
type Credentials struct {
	// Admin credentials
	AdminToken string

	// User credentials
	Email          string
	MasterPassword string

	// OAuth2 credentials
	ClientID     string
	ClientSecret string
}
