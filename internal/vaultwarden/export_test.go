package vaultwarden

import "context"

// DoRequest exposes doRequest to the tests in package vaultwarden_test, which construct
// ciphers the way Bitwarden clients do.
func (c *Client) DoRequest(ctx context.Context, method, path string, reqBody, respBody any) error {
	_, err := c.doRequest(ctx, method, path, reqBody, respBody)
	return err
}

// ProviderUserID exposes providerUserID to the tests in package vaultwarden_test.
func (c *Client) ProviderUserID(ctx context.Context) (string, error) {
	return c.providerUserID(ctx)
}
