# Find the groups of an organization, and generate import blocks with:
#   terraform query -generate-config-out=generated.tf
list "vaultwarden_organization_group" "all" {
  provider = vaultwarden

  config {
    organization_id = "<org_id>"
  }
}

# Only groups whose name contains "admins", ignoring case
list "vaultwarden_organization_group" "admins" {
  provider = vaultwarden

  config {
    organization_id = "<org_id>"
    name_contains   = "admins"
  }
}
