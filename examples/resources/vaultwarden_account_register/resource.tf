# Terraform 1.11 and later: the write-only password is never stored in the plan or state
variable "account_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "vaultwarden_account_register" "example" {
  name                = "Example User"
  email               = "foo@example.com"
  password_wo         = var.account_password
  password_wo_version = 1 # Change to register the account again with the current password
}

# Older Terraform versions: the password is stored in plain text in the state
resource "random_password" "legacy" {
  length = 32
}

resource "vaultwarden_account_register" "legacy" {
  name     = "Legacy User"
  email    = "bar@example.com"
  password = random_password.legacy.result
}
