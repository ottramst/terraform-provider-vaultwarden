data "vaultwarden_organization_group" "by_name" {
  organization_id = "a1b2c3d4-0000-0000-0000-000000000000"
  name            = "Engineering"
}

data "vaultwarden_organization_group" "by_id" {
  organization_id = "a1b2c3d4-0000-0000-0000-000000000000"
  id              = "e5f6a7b8-0000-0000-0000-000000000000"
}
