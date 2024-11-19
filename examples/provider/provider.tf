provider "vaultwarden" {
  endpoint = "https://vaultwarden.example.com"

  # Required: User Authentication
  email           = "your-email-here"
  master_password = "your-master-password-here"

  # Optional: API Authentication (OAuth2)
  # When using OAuth2, user authentication above is still required
  # client_id     = "your-client-id"
  # client_secret = "your-client-secret"

  # Optional: Admin Authentication
  # Required only for /admin endpoint operations
  # admin_token = "your-token-here"
}
