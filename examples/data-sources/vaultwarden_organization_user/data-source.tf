# Look up a member of an organization by email
data "vaultwarden_organization_user" "by_email" {
  organization_id = "53878c48-51e9-416d-b31a-1b4209c93832"
  email           = "user@example.com"
}

# Or by the ID of the organization user
data "vaultwarden_organization_user" "by_id" {
  organization_id = "53878c48-51e9-416d-b31a-1b4209c93832"
  id              = "0a5f6a36-38e4-4cbb-9a3a-25cbd76de8b5"
}

# Give the user access to a collection
resource "vaultwarden_organization_collection_user" "example" {
  organization_id      = data.vaultwarden_organization_user.by_email.organization_id
  collection_id        = "c3c1d2c5-2f5e-4bb4-9e4b-0e3f3c6f5d52"
  organization_user_id = data.vaultwarden_organization_user.by_email.id
}
