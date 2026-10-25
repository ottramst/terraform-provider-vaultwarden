# Find the members of an organization, and generate import blocks with:
#   terraform query -generate-config-out=generated.tf
list "vaultwarden_organization_user" "all" {
  provider = vaultwarden

  config {
    organization_id = "<org_id>"
  }
}

# Only members whose email contains "@example.com", ignoring case
list "vaultwarden_organization_user" "example_com" {
  provider = vaultwarden

  config {
    organization_id = "<org_id>"
    email_contains  = "@example.com"
  }
}
