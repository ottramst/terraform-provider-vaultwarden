resource "vaultwarden_organization" "example" {
  name = "Example"
}

resource "vaultwarden_organization_collection" "example" {
  organization_id = vaultwarden_organization.example.id
  name            = "Example Collection"
}

resource "vaultwarden_organization_user" "example" {
  organization_id = vaultwarden_organization.example.id
  email           = "user@example.com"
}

resource "vaultwarden_organization_collection_user" "example" {
  organization_id      = vaultwarden_organization.example.id
  collection_id        = vaultwarden_organization_collection.example.id
  organization_user_id = vaultwarden_organization_user.example.id
  read_only            = true
}
