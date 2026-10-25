resource "vaultwarden_organization" "example" {
  name = "Example Organization"
}

# Rotate the key every 90 days
resource "time_rotating" "example" {
  rotation_days = 90
}

resource "vaultwarden_organization_api_key_rotation" "example" {
  organization_id = vaultwarden_organization.example.id

  triggers = {
    rotation = time_rotating.example.id
  }
}

# Read the key after it was rotated
ephemeral "vaultwarden_organization_api_key" "example" {
  organization_id = vaultwarden_organization_api_key_rotation.example.organization_id

  depends_on = [vaultwarden_organization_api_key_rotation.example]
}
