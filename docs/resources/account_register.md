---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "vaultwarden_account_register Resource - vaultwarden"
subcategory: ""
description: |-
  This resource registers a new account on the Vaultwarden server.
  This resource will save the password in plain text to the state! Use caution!
  Requires admin_token to be set in the provider configuration.
---

# vaultwarden_account_register (Resource)

This resource registers a new account on the Vaultwarden server.

This resource will save the password in plain text to the state! Use caution!

Requires `admin_token` to be set in the provider configuration.

## Example Usage

```terraform
resource "random_password" "example" {
  length = 32
}

resource "vaultwarden_account_register" "example" {
  name     = "Example User"
  email    = "foo@example.com"
  password = random_password.example.result
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `email` (String) The email of the account to register
- `password` (String, Sensitive) The password of the account to register

### Optional

- `name` (String) The name of the account to register

### Read-Only

- `id` (String) ID of the registered account
