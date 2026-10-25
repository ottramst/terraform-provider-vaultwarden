resource "vaultwarden_organization" "example" {
  name = "Example"
}

resource "vaultwarden_organization_user" "example" {
  organization_id = vaultwarden_organization.example.id
  email           = "foo@example.com"
  type            = "User"
}

# Confirm the user on the first apply after they accepted the invitation
resource "vaultwarden_organization_user" "confirmed" {
  organization_id = vaultwarden_organization.example.id
  email           = "bar@example.com"
  auto_confirm    = true
}

# Keep a user in the organization, but revoke their access
resource "vaultwarden_organization_user" "revoked" {
  organization_id = vaultwarden_organization.example.id
  email           = "baz@example.com"
  revoked         = true
}
