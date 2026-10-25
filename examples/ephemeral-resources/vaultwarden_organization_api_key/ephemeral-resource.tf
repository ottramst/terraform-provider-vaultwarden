resource "vaultwarden_organization" "example" {
  name = "Example Organization"
}

ephemeral "vaultwarden_organization_api_key" "example" {
  organization_id = vaultwarden_organization.example.id
}

# Pass the key to a write-only attribute, for example to store it in AWS Secrets Manager
# for the Directory Connector, without saving it in the state
resource "aws_secretsmanager_secret_version" "example" {
  secret_id = aws_secretsmanager_secret.example.id
  secret_string_wo = jsonencode({
    client_id     = ephemeral.vaultwarden_organization_api_key.example.client_id
    client_secret = ephemeral.vaultwarden_organization_api_key.example.client_secret
  })
  secret_string_wo_version = 1
}
