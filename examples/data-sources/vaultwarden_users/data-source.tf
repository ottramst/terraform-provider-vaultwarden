data "vaultwarden_users" "all" {}

# Users with an account but without two-step login
output "users_without_two_factor" {
  value = [for user in data.vaultwarden_users.all.users : user.email if !user.invited && !user.two_factor_enabled]
}
