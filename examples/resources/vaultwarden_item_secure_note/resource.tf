resource "vaultwarden_item_secure_note" "example" {
  name  = "Recovery instructions"
  notes = <<-EOT
    1. Restore the latest backup.
    2. Rotate the credentials.
  EOT
}
