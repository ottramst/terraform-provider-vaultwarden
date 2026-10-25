data "vaultwarden_organization_collections" "example" {
  organization_id = "53878c48-51e9-416d-b31a-1b4209c93832"
}

# The names of all collections
output "collection_names" {
  value = data.vaultwarden_organization_collections.example.collections[*].name
}
