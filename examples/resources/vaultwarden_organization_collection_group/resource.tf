resource "vaultwarden_organization" "example" {
  name = "Example"
}

resource "vaultwarden_organization_collection" "example" {
  organization_id = vaultwarden_organization.example.id
  name            = "Example Collection"
}

resource "vaultwarden_organization_group" "example" {
  organization_id = vaultwarden_organization.example.id
  name            = "Engineering"
}

resource "vaultwarden_organization_collection_group" "example" {
  organization_id = vaultwarden_organization.example.id
  collection_id   = vaultwarden_organization_collection.example.id
  group_id        = vaultwarden_organization_group.example.id
  read_only       = true
}
