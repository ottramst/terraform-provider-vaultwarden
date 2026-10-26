# The events of the last 24 hours. Needs ORG_EVENTS_ENABLED=true on the server
data "vaultwarden_organization_events" "example" {
  organization_id = "a1b2c3d4-0000-0000-0000-000000000000"
  start           = timeadd(plantimestamp(), "-24h")
}

# The members who exported the organization vault
output "vault_exports" {
  value = [
    for event in data.vaultwarden_organization_events.example.events : {
      date    = event.date
      user_id = event.acting_user_id
    } if event.type_name == "organization_client_exported_vault"
  ]
}
