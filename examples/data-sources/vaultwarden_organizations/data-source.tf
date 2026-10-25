data "vaultwarden_organizations" "example" {}

# The IDs of the organizations the provider user owns
output "owned_organization_ids" {
  value = [for org in data.vaultwarden_organizations.example.organizations : org.id if org.type == "Owner"]
}
