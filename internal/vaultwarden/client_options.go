package vaultwarden

import (
	"fmt"
	"net/http"
)

// ClientOption defines a function type for configuring the Client
type ClientOption func(*Client) error

// WithHTTPClient allows setting a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) error {
		if httpClient == nil {
			return fmt.Errorf("HTTP client cannot be nil")
		}
		c.httpClient = httpClient
		return nil
	}
}

// WithDeviceType sets a custom device type
func WithDeviceType(deviceType string) ClientOption {
	return func(c *Client) error {
		if deviceType == "" {
			return fmt.Errorf("device type cannot be empty")
		}
		c.DeviceInfo.DeviceType = deviceType
		return nil
	}
}

// WithDeviceName sets a custom device name
func WithDeviceName(deviceName string) ClientOption {
	return func(c *Client) error {
		if deviceName == "" {
			return fmt.Errorf("device name cannot be empty")
		}
		c.DeviceInfo.DeviceName = deviceName
		return nil
	}
}

// WithAdminToken sets the admin token for the client
func WithAdminToken(token string) ClientOption {
	return func(c *Client) error {
		c.Credentials.AdminToken = token
		return nil
	}
}

// WithUserCredentials sets the email and master password for the client
func WithUserCredentials(email, masterPassword string) ClientOption {
	return func(c *Client) error {
		c.Credentials.Email = email
		c.Credentials.MasterPassword = masterPassword
		return nil
	}
}

// WithOAuth2Credentials sets the client ID and secret for OAuth2 authentication
func WithOAuth2Credentials(clientID, clientSecret string) ClientOption {
	return func(c *Client) error {
		c.Credentials.ClientID = clientID
		c.Credentials.ClientSecret = clientSecret
		return nil
	}
}
