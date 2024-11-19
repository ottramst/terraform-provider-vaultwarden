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

The Vaultwarden provider supports multiple authentication methods, which can be configured through static credentials or environment variables.

#### Authentication methods

The provider requires one of the following authentication methods for API operations:

1. User Credentials Authentication 
   * Uses email and master password
   * Required for operations that need user context

   ```hcl
   provider "vaultwarden" {
     endpoint        = "https://vaultwarden.example.com"
     email           = "your-email"
     master_password = "your-master-password"
   }
   ```

2. OAuth2 API Key Authentication
    * Uses client ID and client secret
    * Also requires email and master password for encryption/decryption operations
    * Required if the user has 2FA enabled

    ```hcl
    provider "vaultwarden" {
      endpoint        = "https://vault.example.com"
      email           = "your-email"
      master_password = "your-master-password"
      client_id       = "your-client-id"
      client_secret   = "your-client-secret"
    }
    ```

3. Admin Authentication (Optional)
    * Uses admin token
    * Required only for `/admin` endpoint operations
    * Can be combined with either authentication method above

   ```hcl
   provider "vaultwarden" {
     endpoint        = "https://vault.example.com"
      admin_token     = "your-admin-token"
      
      # Required: User credentials
      email           = "user@example.com"
      master_password = "your-secure-password"
      
      # Optional: OAuth2 credentials
      client_id       = "your-client-id"
      client_secret   = "your-client-secret"
   }
   ```

#### Important notes

* If user credentials are used, `email` and `master_password` are always required
* Admin token is optional and can be combined with either authentication method
* Without admin token, `/admin` endpoint operations will not be available

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

All credentials can be provided via environment variables:

```shell
# Required: Endpoint URL
export VAULTWARDEN_ENDPOINT="https://vault.example.com"

# Method 1: User Credentials
export VAULTWARDEN_EMAIL="user@example.com"
export VAULTWARDEN_MASTER_PASSWORD="your-secure-password"

# Method 2: OAuth2 Credentials
export VAULTWARDEN_CLIENT_ID="your-client-id"
export VAULTWARDEN_CLIENT_SECRET="your-client-secret"

# Optional: Admin Token
export VAULTWARDEN_ADMIN_TOKEN="your-admin-token"
```

* Provide the endpoint URL via the `VAULTWARDEN_ENDPOINT` environment variable
* Provide the admin token via the `VAULTWARDEN_ADMIN_TOKEN` environment variable
* Provide the client credentials via the `VAULTWARDEN_EMAIL` and `VAULTWARDEN_MASTER_PASSWORD` environment variables
* Provide the client credentials via the `VAULTWARDEN_CLIENT_ID` and `VAULTWARDEN_CLIENT_SECRET` environment variables

When using environment variables, you can use a minimal provider configuration:

```hcl
provider "vaultwarden" {}
```

#### Authentication Method Selection

The provider validates that you're using exactly one authentication method:

✅ Valid Configurations:

```hcl
# User credentials only
provider "vaultwarden" {
   endpoint        = "https://vault.example.com"
   email           = "user@example.com"
   master_password = "your-secure-password"
}

# User credentials with admin token
provider "vaultwarden" {
   endpoint        = "https://vault.example.com"
   email           = "user@example.com"
   master_password = "your-secure-password"
   admin_token     = "your-admin-token"
}

# Full configuration with OAuth2
provider "vaultwarden" {
   endpoint        = "https://vault.example.com"
   email           = "user@example.com"
   master_password = "your-secure-password"
   client_id       = "your-client-id"
   client_secret   = "your-client-secret"
}

# Admin token only
provider "vaultwarden" {
   endpoint    = "https://vault.example.com"
   admin_token = "your-admin-token"
}
```

❌ Invalid Configurations:

```hcl
# Invalid: Missing user credentials
provider "vaultwarden" {
   endpoint      = "https://vault.example.com"
   client_id     = "your-client-id"
   client_secret = "your-client-secret"
}

# Invalid: Missing master password
provider "vaultwarden" {
   endpoint    = "https://vault.example.com"
   email       = "user@example.com"
}
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
