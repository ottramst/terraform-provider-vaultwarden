# A login in the personal vault of the provider user
resource "vaultwarden_folder" "infrastructure" {
  name = "Infrastructure"
}

resource "vaultwarden_item_login" "database" {
  folder_id = vaultwarden_folder.infrastructure.id
  name      = "Database"
  username  = "admin"
  password  = var.database_password
  totp      = var.database_totp_secret
  notes     = "Primary database"

  uris = [
    { uri = "https://db.example.com", match = "host" },
  ]

  fields = [
    { name = "Port", value = "5432" },
    { name = "Recovery code", value = var.database_recovery_code, type = "hidden" },
  ]
}

# A login shared through an organization collection
resource "vaultwarden_organization" "example" {
  name = "Example"
}

resource "vaultwarden_organization_collection" "operations" {
  organization_id = vaultwarden_organization.example.id
  name            = "Operations"
}

resource "vaultwarden_item_login" "shared" {
  organization_id = vaultwarden_organization.example.id
  collection_ids  = [vaultwarden_organization_collection.operations.id]
  name            = "Monitoring"
  username        = "ops"
  password        = var.monitoring_password
}
