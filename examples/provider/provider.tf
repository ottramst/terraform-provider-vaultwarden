provider "vaultwarden" {
  endpoint = "https://vaultwarden.example.com"

  # Provide at least one of the following authentication methods

  # Optional: Admin Authentication
  # Required only for /admin (admin page) endpoint operations
  admin_token = "your-token-here"

  # Optional: User Authentication (credentials)
  email           = "your-email-here"
  master_password = "your-master-password-here"

  # Optional: API Authentication (OAuth2)
  # When using OAuth2, user credentials above are still required
  client_id     = "your-client-id"
  client_secret = "your-client-secret"
}
