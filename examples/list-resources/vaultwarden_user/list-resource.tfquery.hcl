# Find all users of the server, and generate import blocks with:
#   terraform query -generate-config-out=generated.tf
list "vaultwarden_user" "all" {
  provider = vaultwarden
}

# Only users whose email contains "@example.com", ignoring case
list "vaultwarden_user" "example_com" {
  provider = vaultwarden

  config {
    email_contains = "@example.com"
  }
}
