# Terraform Provider for Vaultwarden

[![CI/CD](https://github.com/ottramst/terraform-provider-vaultwarden/actions/workflows/main.yml/badge.svg?branch=main)](https://github.com/ottramst/terraform-provider-vaultwarden/actions/workflows/main.yml)
[![CodeQL](https://github.com/ottramst/terraform-provider-vaultwarden/actions/workflows/codeql.yml/badge.svg?branch=main)](https://github.com/ottramst/terraform-provider-vaultwarden/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/ottramst/terraform-provider-vaultwarden/badge)](https://scorecard.dev/viewer/?uri=github.com/ottramst/terraform-provider-vaultwarden)
[![Terraform Registry](https://img.shields.io/badge/terraform-registry-623CE4)](https://registry.terraform.io/providers/ottramst/vaultwarden/latest)

This [Terraform](https://www.terraform.io) provider is built using the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework) and allows you to manage [Vaultwarden](https://github.com/dani-garcia/vaultwarden) resources. Vaultwarden is an unofficial Bitwarden-compatible server written in Rust.

This provider includes:

- Resources and data sources for managing Vaultwarden entities (`internal/provider/`)
- A Vaultwarden API client with the client-side encryption Bitwarden clients use (`internal/vaultwarden/`)
- Examples (`examples/`) and generated documentation (`docs/`)
- Acceptance tests for resources and data sources

## Features

- **Organizations**: Manage organizations and read them with a data source
- **Collections**: Manage organization collections, with names encrypted client-side using the organization key
- **Members & access**: Invite users to organizations with a role (`Owner`, `Admin`, `Manager`, `User`), and give them access to individual collections
- **Server users** (admin API): Invite users to the server, and register accounts with a master password
- **Authentication**: Log in with the master password or the user's personal API key, and use the admin token for `/admin` operations

For the complete list of resources and data sources with detailed documentation, see the [docs/](docs/) directory.

## Vaultwarden version compatibility

Acceptance tests run in CI against **Vaultwarden 1.36.0** and **1.37.4** (both legs are required to pass) across Terraform 1.16 and 1.17.

### Support policy

Vaultwarden only ships fixes for its newest release, so the provider fully supports, and tests the latest patch of, the two newest Vaultwarden minor releases:

| Release line | Provider support |
| --- | --- |
| 1.37.x | Supported, tested in CI |
| 1.36.x | Supported, tested in CI |
| 1.35.x and older | Best effort |

*Best effort* means the provider may still work, but these versions are not tested in CI, and a bug that only reproduces on them may not be fixed. Upgrading Vaultwarden is the recommended remedy.

Terraform follows the same policy. HashiCorp supports the two newest Terraform minor releases, currently **1.17** and **1.16**, and CI tests those. The provider uses plugin protocol 6, so it still works with older Terraform 1.x releases, but EOL Terraform versions are supported on a best-effort basis only.

At configure time the provider makes an unauthenticated `GET {endpoint}/api/version` request and warns when the server is older than the supported versions. The probe never fails provider configuration.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0 (protocol 6). CI tests the non-EOL releases, currently 1.16 and 1.17; older versions get best-effort support (see [support policy](#support-policy))
- [Go](https://golang.org/doc/install) >= 1.27 (only required to build the provider from source)
- [Vaultwarden](https://github.com/dani-garcia/vaultwarden) 1.36.x or 1.37.x; older versions on a best-effort basis (see [support policy](#support-policy)). Resources that use the admin API need the [admin page](https://github.com/dani-garcia/vaultwarden/wiki/Enabling-admin-page) enabled

## Building The Provider

1. Clone the repository
1. Enter the repository directory
1. Build the provider using the Go `install` command:

```shell
go install
```

## Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.

## Using the Provider

```hcl
terraform {
  required_providers {
    vaultwarden = {
      source  = "ottramst/vaultwarden"
      version = "~> 0.5"
    }
  }
}

provider "vaultwarden" {
  endpoint        = "https://vaultwarden.example.com"
  email           = "user@example.com"
  master_password = var.master_password
  admin_token     = var.admin_token # only for resources that use the admin API
}
```

For usage examples and comprehensive documentation, including all authentication options and environment variables, please refer to the:
- [Provider documentation](docs/index.md) for configuration details
- [Resource documentation](docs/resources/) for available resources
- [Data source documentation](docs/data-sources/) for available data sources
- [Example configurations](examples/) for complete working examples

## Developing the Provider

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

To generate or update documentation, run `make generate`.

### Running Acceptance Tests

Acceptance tests require a running Vaultwarden instance. A `docker-compose.yml` is provided; `VW_VERSION` selects the image tag and defaults to the newest supported version.

```shell
# Start a local Vaultwarden (VW_VERSION=1.36.0 for the older supported version)
docker compose up -d --wait

make testacc

docker compose down
```

The tests use `http://localhost:8000` by default; set `VAULTWARDEN_ENDPOINT` to use another instance. That instance needs the admin token `admin_token` and generous `ADMIN_RATELIMIT_*`, `LOGIN_RATELIMIT_*` and `UNAUTHENTICATED_RATELIMIT_*` limits (see `docker-compose.yml`).

*Note:* Acceptance tests create real users and organizations in your Vaultwarden instance. Make sure to use a test instance and not a production environment.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and how to run the acceptance tests, [SUPPORT.md](SUPPORT.md) for getting help, and [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## Acknowledgements

Big thanks to [maxlaverse](https://github.com/maxlaverse) - we borrowed some of the crypto and other parts of the code from their excellent [terraform-provider-bitwarden](https://github.com/maxlaverse/terraform-provider-bitwarden) project.
It saved us from having to reinvent the wheel with all that cryptography stuff! 🔐
