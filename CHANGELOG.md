## Unreleased

BUG FIXES:

* resource/vaultwarden_organization_user: Fixed "Provider produced inconsistent result after apply" for `access_all` on Vaultwarden 1.34 and later ([#41](https://github.com/ottramst/terraform-provider-vaultwarden/issues/41)). Vaultwarden now decides `access_all` from the user type: `Owner` and `Admin` always have it, a `Manager` has it with the *Manage all collections* permission, and a `User` never has it. `access_all` now defaults from the type, and setting it to `true` for a `User` (or `false` for an `Owner` or `Admin`) is rejected at plan time. Setting `access_all = true` on a `Manager` is now applied on newer Vaultwarden versions too
* resource/vaultwarden_organization_user: Managers are no longer read back as type `Unknown` on Vaultwarden 1.33 and later
* resource/vaultwarden_organization_user: Updates now read the user back from the server instead of copying the configuration into state
* resource/vaultwarden_account_register: Registering accounts works again on Vaultwarden 1.36 and later, which removed the `/api/accounts/register` endpoint. The provider now uses the `/identity/accounts/register/send-verification-email` and `/identity/accounts/register/finish` flow. Registration fails with a clear error when the server requires email verification (`SIGNUPS_VERIFY`)
* resource/vaultwarden_user: Deleting a user works again on Vaultwarden 1.37 and later
* security: Updated `google.golang.org/grpc` to v1.84.0 and `golang.org/x/crypto` to v0.58.0

NOTES:

* Vaultwarden support policy: the provider now supports and tests only the two newest Vaultwarden minor releases, currently 1.36.x and 1.37.x (previously 1.28 to 1.32). Older versions get best-effort support. See the README "Support policy" section
* provider: The server version is detected with `GET /api/version` at configure time, and the provider warns when it is older than the supported versions
* Terraform support policy: CI now tests only the non-end-of-life Terraform releases, currently 1.16 and 1.17 (previously 1.8 and 1.9). The provider still works with older Terraform 1.x releases on a best-effort basis
* tests: Acceptance tests run against a `docker-compose.yml` Vaultwarden (`VW_VERSION` selects the version) and read the endpoint from `VAULTWARDEN_ENDPOINT`. `Makefile` was replaced by a `GNUmakefile` without the `docker-*` targets
* build: The provider now requires Go 1.27 to build from source (Go 1.23 is end-of-life). Dependencies were updated, including `terraform-plugin-framework` v1.19.0 and `terraform-plugin-testing` v1.16.0
* build: golangci-lint configuration migrated to v2, and golangci-lint is pinned to v2.15.0 in CI

## v0.4.4

* Fix issues with `vaultwarden_organization_user` resource attributes on update

## v0.4.3

* Add `access_all` attribute to `vaultwarden_organization_user` resource
* Add proper update logic to `vaultwarden_organization_user` resource
* Update tests for `vaultwarden_organization_user` resource

## v0.4.2

* Fix bug where all auth methods were being required for the client

## v0.4.1

* Add `ResourceWithConfigure` to all resources
* Make sure at least one type of authentication is set for the client
* Fix bug where the `type` field was not being set on the `vaultwarden_organization_user` resource

## v0.4.0

* Add `vaultwarden_account_register` resource
* Add `vaultwarden_organization_user` resource 

## v0.3.0

* Add API key support for Vaultwarden client
* Add `vaultwarden_organization_collection` resource

## v0.2.0

* Add `vaultwarden_organization` resource
* Add `vaultwarden_organization` data source

## v0.1.0

* Initial set of resources
* GitHub Actions
