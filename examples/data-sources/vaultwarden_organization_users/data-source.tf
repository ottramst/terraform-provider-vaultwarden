data "vaultwarden_organization_users" "example" {
  organization_id = "53878c48-51e9-416d-b31a-1b4209c93832"
}

# The emails of the users that have not accepted their invitation yet
output "invited_users" {
  value = [for user in data.vaultwarden_organization_users.example.users : user.email if user.status == "Invited"]
}
