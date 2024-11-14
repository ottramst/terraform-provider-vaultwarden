package vaultwarden

// Option defines the type for client configuration options
type Option func(*Client) error

// WithCredentials sets the username and password for the client
func WithCredentials(email, masterPassword string) Option {
	return func(c *Client) error {
		c.email = email
		c.masterPassword = masterPassword
		return nil
	}
}

// WithAdminToken sets the admin token for the client
func WithAdminToken(token string) Option {
	return func(c *Client) error {
		c.adminToken = token
		return nil
	}
}
