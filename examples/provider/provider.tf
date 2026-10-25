# User authentication, for organizations, collections and organization users
provider "vaultwarden" {
  endpoint        = "https://vaultwarden.example.com"
  email           = "user@example.com"
  master_password = var.master_password
}

# User authentication with the user's personal API key, when two-step login
# is enabled. The master password is still needed to decrypt the user's keys.
provider "vaultwarden" {
  endpoint        = "https://vaultwarden.example.com"
  email           = "user@example.com"
  master_password = var.master_password
  client_id       = var.client_id
  client_secret   = var.client_secret
}

# Admin authentication, for vaultwarden_user and vaultwarden_account_register.
# It can be combined with user authentication.
provider "vaultwarden" {
  endpoint    = "https://vaultwarden.example.com"
  admin_token = var.admin_token
}

# Every attribute can also be set with an environment variable:
# VAULTWARDEN_ENDPOINT, VAULTWARDEN_EMAIL, VAULTWARDEN_MASTER_PASSWORD,
# VAULTWARDEN_CLIENT_ID, VAULTWARDEN_CLIENT_SECRET, VAULTWARDEN_ADMIN_TOKEN
