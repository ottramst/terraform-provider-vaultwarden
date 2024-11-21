---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "vaultwarden_user Resource - vaultwarden"
subcategory: ""
description: |-
  This resource invites a user to the Vaultwarden server.
---

# vaultwarden_user (Resource)

This resource invites a user to the Vaultwarden server.

## Example Usage

```terraform
resource "vaultwarden_user" "example" {
  email = "foo@example.com"
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `email` (String) The email of the user to invite

### Read-Only

- `id` (String) ID of the user

## Import

Import is supported using the following syntax:

```shell
terraform import vaultwarden_user.example <id>
```