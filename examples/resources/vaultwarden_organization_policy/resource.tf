resource "vaultwarden_organization" "example" {
  name = "Example"
}

# A policy with settings
resource "vaultwarden_organization_policy" "master_password" {
  organization_id = vaultwarden_organization.example.id
  type            = "master_password"
  enabled         = true

  data = jsonencode({
    minComplexity  = 3
    minLength      = 14
    requireUpper   = true
    requireLower   = true
    requireNumbers = true
    requireSpecial = true
  })
}

# A policy without settings
resource "vaultwarden_organization_policy" "disable_send" {
  organization_id = vaultwarden_organization.example.id
  type            = "disable_send"
  enabled         = true
}
