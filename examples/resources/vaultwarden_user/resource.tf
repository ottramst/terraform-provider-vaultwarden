resource "vaultwarden_user" "example" {
  email = "foo@example.com"
}

# Keep the user, but block their logins
resource "vaultwarden_user" "disabled" {
  email   = "bar@example.com"
  enabled = false
}
