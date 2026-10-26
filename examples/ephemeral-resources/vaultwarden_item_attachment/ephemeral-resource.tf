# Read an attachment without saving its content in the plan or state (Terraform 1.10 and later)
ephemeral "vaultwarden_item_attachment" "service_account" {
  item_id = "a1b2c3d4-0000-0000-0000-000000000000"
  id      = "e5f6a7b8-0000-0000-0000-000000000000"
}

provider "google" {
  credentials = ephemeral.vaultwarden_item_attachment.service_account.content
}
