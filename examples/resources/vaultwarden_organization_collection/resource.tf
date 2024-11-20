resource "vaultwarden_organization" "example" {
  name = "Example"
}

resource "vaultwarden_organization_collection" "example" {
  organization_id = vaultwarden_organization.example.id
  name            = "Example Collection"
}
