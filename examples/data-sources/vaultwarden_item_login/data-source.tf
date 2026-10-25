data "vaultwarden_item_login" "example" {
  id = "a1b2c3d4-0000-0000-0000-000000000000"
}

# The password is saved in the state: prefer the ephemeral resource where possible
output "username" {
  value = data.vaultwarden_item_login.example.username
}
