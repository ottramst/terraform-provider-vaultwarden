ephemeral "vaultwarden_user_api_key" "example" {}

# Log in a second provider configuration with the personal API key
provider "vaultwarden" {
  alias           = "api_key"
  endpoint        = "https://vaultwarden.example.com"
  email           = var.email
  master_password = var.master_password
  client_id       = ephemeral.vaultwarden_user_api_key.example.client_id
  client_secret   = ephemeral.vaultwarden_user_api_key.example.client_secret
}
