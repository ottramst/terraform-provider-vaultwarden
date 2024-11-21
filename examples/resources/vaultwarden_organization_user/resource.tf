resource "vaultwarden_organization" "example" {
  name = "Example"
}

resource "vaultwarden_organization_user" "example" {
  organization_id = vaultwarden_organization.example.id
  email           = "foo@example.com"
  type            = "User"
}
