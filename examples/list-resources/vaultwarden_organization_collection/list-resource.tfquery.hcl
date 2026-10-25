# Find the collections of an organization, and generate import blocks with:
#   terraform query -generate-config-out=generated.tf
list "vaultwarden_organization_collection" "all" {
  provider = vaultwarden

  config {
    organization_id = "<org_id>"
  }
}

# Only collections whose name contains "engineering", ignoring case
list "vaultwarden_organization_collection" "engineering" {
  provider = vaultwarden

  config {
    organization_id = "<org_id>"
    name_contains   = "engineering"
  }
}
