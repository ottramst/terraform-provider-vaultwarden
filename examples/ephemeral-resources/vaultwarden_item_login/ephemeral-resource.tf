# Read a password without saving it in the plan or state (Terraform 1.10 and later)
ephemeral "vaultwarden_item_login" "database" {
  id = "a1b2c3d4-0000-0000-0000-000000000000"
}

provider "postgresql" {
  host     = "db.example.com"
  username = ephemeral.vaultwarden_item_login.database.username
  password = ephemeral.vaultwarden_item_login.database.password
}
