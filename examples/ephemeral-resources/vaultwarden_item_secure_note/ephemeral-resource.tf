# Read a secure note without saving it in the plan or state (Terraform 1.10 and later)
ephemeral "vaultwarden_item_secure_note" "certificate" {
  id = "a1b2c3d4-0000-0000-0000-000000000000"
}

resource "aws_secretsmanager_secret_version" "certificate" {
  secret_id                = aws_secretsmanager_secret.certificate.id
  secret_string_wo         = ephemeral.vaultwarden_item_secure_note.certificate.notes
  secret_string_wo_version = 1
}
