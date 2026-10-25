# Look up a collection by name. The lookup fails when several collections have the name
data "vaultwarden_organization_collection" "by_name" {
  organization_id = "53878c48-51e9-416d-b31a-1b4209c93832"
  name            = "Engineering"
}

# Or by ID
data "vaultwarden_organization_collection" "by_id" {
  organization_id = "53878c48-51e9-416d-b31a-1b4209c93832"
  id              = "c3c1d2c5-2f5e-4bb4-9e4b-0e3f3c6f5d52"
}
