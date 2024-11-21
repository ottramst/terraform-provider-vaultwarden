resource "random_password" "example" {
  length = 32
}

resource "vaultwarden_account_register" "example" {
  name     = "Example User"
  email    = "foo@example.com"
  password = random_password.example.result
}
