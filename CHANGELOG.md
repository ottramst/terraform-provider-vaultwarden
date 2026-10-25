## Unreleased

FEATURES:

* **New Resource:** `vaultwarden_organization_group` - Manage organization groups, with `name`, `access_all` and `external_id` ([#50](https://github.com/ottramst/terraform-provider-vaultwarden/issues/50)). Updates keep the group's members and collection access. Groups require the `ORG_GROUPS_ENABLED=true` server setting
* **New Data Source:** `vaultwarden_organization_group` - Look up an organization group by ID or name ([#50](https://github.com/ottramst/terraform-provider-vaultwarden/issues/50))

## v0.5.1

BUG FIXES:

* resource/vaultwarden_account_register: New accounts are registered with the default KDF settings (PBKDF2-SHA256, 600,000 iterations). They used to be taken from a prelogin of the provider user's email, which has no account when the provider is configured with only `admin_token`. Vaultwarden 1.37 answers such a prelogin with decoy settings, so registration could fail with "PBKDF2 KDF iterations must be at least 100000", or create the account with weak settings such as Argon2id with 3 iterations

## v0.5.0

BREAKING CHANGES:

* resource/vaultwarden_organization_user: `access_all = true` with `type = "User"`, and `access_all = false` with `type = "Owner"` or `"Admin"`, are now rejected at plan time. Vaultwarden 1.34 and later ignore these values, which caused the error in [#41](https://github.com/ottramst/terraform-provider-vaultwarden/issues/41). Remove `access_all`, or give `User` members access to collections with `vaultwarden_organization_collection_user`
* resource/vaultwarden_user, resource/vaultwarden_account_register, resource/vaultwarden_organization_user: Changing `email` (and `name` or `password` of an `account_register`) now plans a replacement, which deletes the user or account, including its vault. Before, the change was never applied. Review plans for `forces replacement` before applying, or use `lifecycle { prevent_destroy = true }`
* Vaultwarden 1.35 and older are no longer tested. See the support policy in the README

FEATURES:

* **New Resource:** `vaultwarden_organization_collection_user` - Give an organization user access to a collection, with `read_only`, `hide_passwords` and `manage` ([#23](https://github.com/ottramst/terraform-provider-vaultwarden/issues/23)). Access of other users and groups to the collection is left unchanged

ENHANCEMENTS:

* resource/vaultwarden_account_register: Support import by ID. The password cannot be read back, and setting it after an import only updates the state
* resource/vaultwarden_account_register: `name` is now computed when not set

BUG FIXES:

* resource/vaultwarden_organization_user: Fixed "Provider produced inconsistent result after apply" for `access_all` on Vaultwarden 1.34 and later ([#41](https://github.com/ottramst/terraform-provider-vaultwarden/issues/41)). Vaultwarden now decides `access_all` from the user type: `Owner` and `Admin` always have it, a `Manager` has it with the *Manage all collections* permission, and a `User` never has it. `access_all` now defaults from the type, and setting it to `true` for a `User` (or `false` for an `Owner` or `Admin`) is rejected at plan time. Setting `access_all = true` on a `Manager` is now applied on newer Vaultwarden versions too
* resource/vaultwarden_organization_user: Managers are no longer read back as type `Unknown` on Vaultwarden 1.33 and later
* resource/vaultwarden_organization_user: Updates now read the user back from the server instead of copying the configuration into state
* resource/vaultwarden_account_register: Registering accounts works again on Vaultwarden 1.36 and later, which removed the `/api/accounts/register` endpoint. The provider now uses the `/identity/accounts/register/send-verification-email` and `/identity/accounts/register/finish` flow. Registration fails with a clear error when the server requires email verification (`SIGNUPS_VERIFY`)
* resource/vaultwarden_user: Deleting a user works again on Vaultwarden 1.37 and later
* provider: Resources deleted outside Terraform are now removed from state and planned for creation, instead of failing the refresh. Deleting a resource that is already gone now succeeds ([#44](https://github.com/ottramst/terraform-provider-vaultwarden/issues/44))
* resource/vaultwarden_organization_user, resource/vaultwarden_organization_collection: Updates no longer remove the collection and group assignments of the user or collection, for example ones made in the web vault ([#45](https://github.com/ottramst/terraform-provider-vaultwarden/issues/45))
* resource/vaultwarden_user, resource/vaultwarden_account_register, resource/vaultwarden_organization_user: Changing `email` (and `name` or `password` of an `account_register`) now replaces the resource. Previously the change was reported as applied but never sent to the server. Replacing a user or a registered account deletes it, including its vault ([#46](https://github.com/ottramst/terraform-provider-vaultwarden/issues/46))
* provider: Fixed a race that could fail a refresh or apply with "organization not found or not authenticated" when several resources triggered a login at the same time. Concurrent requests now wait for a single login, and the admin login no longer affects requests running at the same time
* provider: Creating an organization no longer crashes the provider when key generation fails ([#47](https://github.com/ottramst/terraform-provider-vaultwarden/issues/47))
* security: Updated `google.golang.org/grpc` to v1.84.0 and `golang.org/x/crypto` to v0.58.0

NOTES:

* Vaultwarden support policy: the provider now supports and tests only the two newest Vaultwarden minor releases, currently 1.36.x and 1.37.x (previously 1.28 to 1.32). Older versions get best-effort support. See the README "Support policy" section
* provider: The server version is detected with `GET /api/version` at configure time, and the provider warns when it is older than the supported versions
* Terraform support policy: CI now tests only the non-end-of-life Terraform releases, currently 1.16 and 1.17 (previously 1.8 and 1.9). The provider still works with older Terraform 1.x releases on a best-effort basis
* tests: Acceptance tests run against a `docker-compose.yml` Vaultwarden (`VW_VERSION` selects the version) and read the endpoint from `VAULTWARDEN_ENDPOINT`. `Makefile` was replaced by a `GNUmakefile` without the `docker-*` targets
* build: The provider now requires Go 1.27 to build from source (Go 1.23 is end-of-life). Dependencies were updated, including `terraform-plugin-framework` v1.19.0 and `terraform-plugin-testing` v1.16.0
* build: golangci-lint configuration migrated to v2, and golangci-lint is pinned to v2.15.0 in CI
* ci: All actions are pinned to commit SHAs with least-privilege `permissions`, job timeouts and concurrency groups. CI also runs on pushes to `main`, adds a `go mod tidy` check, unit tests, and a single `CI gate` status check. New workflows: CodeQL, govulncheck and dependency review, OpenSSF Scorecard, and workflow linting (actionlint, zizmor). Releases now publish build provenance attestations
* project: Added `CONTRIBUTING.md`, `SECURITY.md`, `SUPPORT.md`, `CLAUDE.md`, issue forms and a pull request template, replaced the inherited HashiCorp code of conduct with the Contributor Covenant 2.1, and fixed `CODEOWNERS`
* docs: The README follows the same structure as the other providers, and the authentication options and environment variables are now documented in the provider documentation

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
