resource "vaultwarden_organization" "example" {
  name = "Example"
}

resource "vaultwarden_organization_group" "example" {
  organization_id = vaultwarden_organization.example.id
  name            = "Engineering"
}

resource "vaultwarden_organization_user" "example" {
  organization_id = vaultwarden_organization.example.id
  email           = "user@example.com"
}

resource "vaultwarden_organization_group_member" "example" {
  organization_id      = vaultwarden_organization.example.id
  group_id             = vaultwarden_organization_group.example.id
  organization_user_id = vaultwarden_organization_user.example.id
}
