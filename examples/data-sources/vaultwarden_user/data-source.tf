# Look up a user by email
data "vaultwarden_user" "by_email" {
  email = "user@example.com"
}

# Or by ID
data "vaultwarden_user" "by_id" {
  id = "5b7c2f2e-3c4e-4a39-9a51-2f0c4c7d8e91"
}

output "user_has_two_factor" {
  value = data.vaultwarden_user.by_email.two_factor_enabled
}
