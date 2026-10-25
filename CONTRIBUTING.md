# Contributing

Thanks for your interest in improving the Vaultwarden Terraform provider. Bug reports, feature
requests and pull requests are all welcome. By participating you agree to follow the
[Code of Conduct](.github/CODE_OF_CONDUCT.md).

## Reporting issues

Use the [issue templates](https://github.com/ottramst/terraform-provider-vaultwarden/issues/new/choose).
Always include the provider, Terraform and Vaultwarden versions. Report security vulnerabilities
privately, as described in [SECURITY.md](SECURITY.md).

## Development setup

Requirements: [Go](https://go.dev/doc/install) (the version in `go.mod`), [Terraform](https://developer.hashicorp.com/terraform/install),
Docker, and [golangci-lint](https://golangci-lint.run/) (the version pinned in `.github/workflows/lint.yml`).

```shell
make build      # build the provider
make test       # unit tests
make lint       # golangci-lint
make generate   # regenerate docs/ from the schemas and examples/
```

`make generate` must produce no diff in CI, so commit the regenerated `docs/` with every schema or
example change. Never edit `docs/` by hand: change the schema descriptions or `examples/` instead.

## Acceptance tests

Acceptance tests run against a real Vaultwarden instance. A Docker Compose file is provided;
`VW_VERSION` selects the image tag and defaults to the newest supported version.

```shell
docker compose up -d --wait
# VW_VERSION=1.36.0 docker compose up -d --wait

make testacc
# or a single test:
TF_ACC=1 go test -v -run TestAccOrganizationUser ./internal/provider/ -timeout 120m

docker compose down
```

The tests register and log in as `test@example.com` and use the admin token `admin_token`. Set
`VAULTWARDEN_ENDPOINT` to run them against another instance; it needs that admin token and generous
`ADMIN_RATELIMIT_*`, `LOGIN_RATELIMIT_*` and `UNAUTHENTICATED_RATELIMIT_*` limits (see `docker-compose.yml`).

Behavior that differs between Vaultwarden versions must be covered for each supported version. Run the
suite against every version in the test matrix before opening a pull request, and prefer detecting
what the server returns over checking its version.

## Supported Vaultwarden versions

CI tests the latest patch of the two newest Vaultwarden minor releases (see the
[support policy](README.md#support-policy)). When upstream ships a new release, bump these together:

1. The matrix in `.github/workflows/test.yml`
2. The `VW_VERSION` default in `docker-compose.yml`
3. `MinimumSupportedVersion` in `internal/vaultwarden/version.go`, when a new minor release ships
4. The tested versions in `README.md`, the provider description in `internal/provider/provider.go`, and `CLAUDE.md`

The same applies to Terraform: the `terraform-version` matrix in `.github/workflows/test.yml` covers the two
newest Terraform minor releases (the ones HashiCorp supports). When a new minor ships, add it, drop the oldest,
and update the versions in `README.md` and `CLAUDE.md`.

## Pull requests

- Keep each pull request focused on one change, and add or update acceptance tests for it.
- Add an entry to `CHANGELOG.md` under `## Unreleased` (`FEATURES`, `ENHANCEMENTS`, `BUG FIXES` or `NOTES`).
- CI must pass: the `CI gate` check covers docs generation, lint, build, unit tests and the acceptance
  test matrix.

## Releases

Maintainers release by pushing a `vX.Y.Z` tag. The release workflow builds and signs the artifacts with
GoReleaser, attests their build provenance, and publishes a GitHub release that the Terraform Registry picks up.
Before tagging, move the `## Unreleased` entries in `CHANGELOG.md` under the new version.
