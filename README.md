# Terraform Provider Vaultwarden

![Build Status](https://github.com/ottramst/terraform-provider-vaultwarden/actions/workflows/main.yml/badge.svg)

## Use of the provider

The Vaultwarden provider allows you to manage and configure [Vaultwarden](https://github.com/dani-garcia/vaultwarden), a Bitwarden server API implementation in Rust, using Terraform.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.8
- [Go](https://golang.org/doc/install) >= 1.23

## Getting started

The provider supports Vaultwarden versions:
* 1.32.x
* 1.32.x
* 1.31.x
* 1.30.x
* 1.29.x
* 1.28.x
* 1.27.x
* 1.26.x
* 1.25.x

For proper provider functionality, your Vaultwarden instance must have admin access configured. See Vaultwarden's Admin Page documentation for setup instructions.
Access to the admin API is granted via passing the `admin_token` to the provider configuration. If this is not provided, the provider will not be able to manage the `/admin` endpoints.

### Requiring the provider

```hcl
terraform {
  required_version = ">= 1.8"
  
  required_providers {
    vaultwarden = {
      source  = "ottramst/vaultwarden"
      version = "~> 0.1"
    }
  }
}
```

### Authentication

The Vaultwarden provider supports the following methods of authentication:

* Static credentials in the provider configuration
* Environment variables

There are two types of authentication for this provider, neither one is required, but at least one must be provided.

1. Admin token - for managing the `/admin` endpoints
2. User credentials - for managing the `/api` endpoints

#### Static credentials

```hcl
provider "vaultwarden" {
  endpoint      = "https://vaultwarden.example.com"

  # Either use admin token
  admin_token   = "your-admin-token"

  # Or use credentials
  email           = "your-email"
  master_password = "your-master-password"

  # Or use both if needed
}
```

#### Environment variables

* Provide the endpoint URL via the `VAULTWARDEN_ENDPOINT` environment variable
* Provide the admin token via the `VAULTWARDEN_ADMIN_TOKEN` environment variable
* Provide the client credentials via the `VAULTWARDEN_EMAIL` and `VAULTWARDEN_MASTER_PASSWORD` environment variables

And use the provider configuration like this:

```hcl
provider "vaultwarden" {}
```

## Developing the provider

1. Clone the repository
2. Enter the repository directory
3. Build the provider using the Go `install` command:

```shell
go install .
```

4. Make sure you override your `.terraformrc` file with the following content:

```hcl
provider_installation {

  dev_overrides {
      "ottramst/vaultwarden" = "<GO_BIN_PATH>"
  }

  # For all other providers, install them directly from their origin provider
  # registries as normal. If you omit this, Terraform will _only_ use
  # the dev_overrides block, and so no other providers will be available.
  direct {}
}
```

### Updating documentation and examples

If adding a new resource, data source or a function, make sure to update the documentation and examples in the `examples` directory.

Generate the documentation using the following command:

```shell
make generate
```

### Running acceptance tests

Acceptance tests require Vaultwarden server to be running.
Makefile is provided to run the acceptance tests:

```shell
make docker-testacc
```

## Support

This provider is maintained by the community. Issues and feature requests can be filed on the [GitHub repository](https://github.com/ottramst/terraform-provider-vaultwarden/issues).

### Support Scope

While we welcome bug reports and feature requests, please note:

* The provider is community-maintained
* We cannot guarantee immediate responses to issues or feature requests
* For urgent production issues, it's recommended to use the Vaultwarden web interface directly

## Documentation

Full provider documentation is available on the [Terraform Registry](https://registry.terraform.io/providers/ottramst/vaultwarden/latest).

## Acknowledgements

Big thanks to [maxlaverse](https://github.com/maxlaverse) - we borrowed some of the crypto and other parts of the code from their excellent [terraform-provider-bitwarden](https://github.com/maxlaverse/terraform-provider-bitwarden) project.
It saved us from having to reinvent the wheel with all that cryptography stuff! 🔐
