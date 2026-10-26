# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Terraform provider for [Vaultwarden](https://github.com/dani-garcia/vaultwarden), an unofficial Bitwarden-compatible server, built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework). Provider code lives in `internal/provider/`; the Vaultwarden API client, including the Bitwarden client-side cryptography, lives in `internal/vaultwarden/`. Parts of the crypto code come from [maxlaverse/terraform-provider-bitwarden](https://github.com/maxlaverse/terraform-provider-bitwarden), a good reference for Bitwarden API behavior.

The provider supports and tests the two newest Vaultwarden minor releases (currently 1.36.x and 1.37.x). Older versions get best-effort support: don't add CI legs for them. Vaultwarden has no versioned API and changes behavior between minor releases, so check new code against every supported version, preferably by detecting what the server returns rather than by checking its version.

## Common Commands

```bash
make default          # fmt, lint, install, generate (full dev cycle)
make build            # Build binary
make lint             # Run golangci-lint
make fmt              # Format Go code with gofmt
make generate         # Generate provider documentation (tfplugindocs) + terraform fmt examples
make test             # Unit tests (120s timeout, parallel=10)
make testacc          # Acceptance tests (requires running Vaultwarden instance)
```

Run a single test:
```bash
TF_ACC=1 go test -v -run TestAccOrganizationUser ./internal/provider/ -timeout 120m
```

## Running Acceptance Tests Locally

Acceptance tests require a running Vaultwarden instance:

```bash
# 1. Start Vaultwarden on http://localhost:8000 with the admin token "admin_token"
#    and relaxed rate limits. VW_VERSION selects the version (default: newest supported).
docker compose up -d --wait
# VW_VERSION=1.36.0 docker compose up -d --wait

# 2. Run all acceptance tests (add -race to also check the client for data races)
make testacc

# Or run a specific test
TF_ACC=1 go test -v -run TestAccOrganizationUser ./internal/provider/ -timeout 120m

# 3. Tear down when done (data is on a tmpfs, so it is gone afterwards)
docker compose down
```

The tests register `test@example.com` on first use (`internal/vaultwarden/test/helpers.go`) and read the endpoint from `VAULTWARDEN_ENDPOINT` (default `http://localhost:8000`). Vaultwarden rate-limits logins, unauthenticated endpoints (prelogin, registration) and admin logins per IP; the compose file raises all three limits, and any other instance needs the same.

## Architecture

**Provider entry point:** `main.go` → `internal/provider/provider.go`

`Configure()` builds a `*vaultwarden.Client` from the provider attributes (or `VAULTWARDEN_*` environment variables), probes `GET /api/version` and warns when the server is older than `vaultwarden.MinimumSupportedVersion` (`version.go`). The probe never fails configuration. The client is passed to every resource and data source through `Configure()`.

**Client (`internal/vaultwarden/`):**
- Authentication (`auth*.go`): user login with the master password or the personal API key (`/identity/connect/token`), and admin login with the admin token (`POST /admin`, cookie session). Logins happen lazily on the first request and are serialized by `authMu`: use `userSession()` / `UserKey()` / `adminCookie()` to get a valid session; never read `AuthState` fields directly.
- Organization keys are decrypted at login from the profile and cached; read them with `Client.OrganizationSecret(orgID)`. Collection names are encrypted with the organization key (`crypt`, `encryptedstring`, `keybuilder`, `symmetrickey` packages).
- The user's symmetric key (the "user key", decrypted at login from the token response's `Key` with the master key) encrypts the personal vault, such as folder names. Read it with `Client.UserKey(ctx)`, which logs in if needed. `encryptString`/`decryptString` (`encryption.go`) map empty values to an empty string, because Bitwarden clients send null instead of encrypting an empty string, and `crypt.Encrypt` rejects empty input.
- Folders (`folder.go`): the API is the same on 1.36 and 1.37. A missing folder, or one of another user, is a 400 "Invalid folder" (a 404 is also treated as not found). Deleting a folder keeps its items.
- Items (`item.go`, Vaultwarden "ciphers"): a cipher's fields are encrypted with its item key (cipher key, the `key` attribute, a 64-byte key encrypted with the user or organization key), or directly with the owner's key when `key` is null (older clients). Reading supports both; new items always get an item key, and updates keep the existing key (or its absence), so that unmanaged encrypted data (password history, passkeys, attachment keys) stays readable. Updates start from the raw JSON of the cipher and only replace managed fields. Login URIs need `uriChecksum` (encrypted base64 SHA-256 of the URI): Bitwarden clients drop URIs without a valid checksum when the cipher has a key, and so does the provider when reading. Vaultwarden 1.37 requires `encryptedFor` (the provider user's ID, `AuthState.UserID`) on every cipher write and unarchives a cipher whose `archivedDate` is not sent; both are harmless on 1.36. Organization items are created with `POST /ciphers/create` and their collections changed with `PUT /ciphers/{id}/collections_v2`. A missing cipher is a 400 ("Cipher doesn't exist"), and items in the trash (`deletedDate`) are treated as not found. Organization keys missing from the cache (an organization created by another client since the login) are read from the profile again (`refreshOrganizationSecrets`).
- Attachments (`attachment.go`): `POST /api/ciphers/{id}/attachment/v2` registers the attachment (`key`, `fileName`, `fileSize` of the encrypted file), then the file is uploaded as multipart field `data` to `/api/ciphers/{id}/attachment/{attachmentId}` (`multipartFile` in `send()`). The file is encrypted with a new 64-byte attachment key and sent as an encrypted buffer (`EncryptedString.Buffer()`: type byte, IV, MAC, data); the file name and the attachment key are encrypted with the item key. Vaultwarden builds the download URL from its `DOMAIN`, so `/attachments/...` URLs are rebased onto the provider endpoint (`attachmentDownloadPath`); a `*[]byte` response body returns raw bytes. A missing attachment is a 400 "Attachment doesn't exist". Empty files cannot be encrypted, and 1.37 rejects files over 525 MB. Item updates don't send `attachments2`, which keeps the attachments as they are.
- Ephemeral resources get the client from `EphemeralResourceData`; their acceptance tests use the `echo` provider from `terraform-plugin-testing/echoprovider` and need Terraform 1.10+. `internal/vaultwarden/item_test.go` (package `vaultwarden_test`, `export_test.go`) builds ciphers the way Bitwarden clients do, with keys derived from the master password, to check the round trip.
- Requests go through `send()` (`client.go`), which retries rate limited requests (HTTP 429) with exponential backoff for up to `max_retry_wait` (default 60s); Vaultwarden sends no `Retry-After` header, and the final error names the server setting of the limiter that fired. Non-2xx responses return an `*APIError`. Methods map Vaultwarden's not-found responses to a wrapped `ErrNotFound` (`IsNotFound(err)`); they differ per object: a JSON 404 for admin users, a 400 for organization members, a missing list entry for collections, a 400 with "Group not found" for groups, and a generic 401 for a deleted organization, confirmed against the user's profile (`organizationGone`).
- Vaultwarden's endpoints for collection and member assignments replace all assignments. Updates read the current assignments and send them back, and all writes to assignments in an organization are serialized with `lockOrganizationAccess(orgID)`. Collection access is written with `POST /collections/bulk-access`; the per-collection user endpoints were removed in Vaultwarden 1.36.
- Groups need `ORG_GROUPS_ENABLED=true` on the server (set in `docker-compose.yml`); otherwise group endpoints answer 400 "Group support is disabled" (`ErrGroupsDisabled`), and list endpoints return no groups. The group `PUT` replaces the group's collections and members, and ignores `externalId`. Vaultwarden 1.37 no longer returns a group's `accessAll`, so group members are changed with the member `PUT` (`setOrganizationUserGroups`), not the group `PUT`; 1.37 also removed `PUT /groups/{id}/users`.
- Organization events (`event.go`, `GET /api/organizations/{id}/events`) need `ORG_EVENTS_ENABLED=true` (set in `docker-compose.yml`); otherwise Vaultwarden logs nothing and returns an empty list. Pages hold 30 events, newest first; `continuationToken` is the date of the last one and is queried inclusively, so events of the previous page at that date are skipped. `start` and `end` are RFC 3339 and required. Needs the `Admin` or `Owner` role.
- Endpoints that confirm an action with the master password (deleting an organization, reading or rotating API keys) take the body from `passwordVerification()` (`MasterPasswordHash()`, `api_key.go`). The organization API key endpoints need the `Owner` role on 1.37 (1.36 also allows `Admin`), and return only the secret: the client ID is `organization.<org_id>` (`user.<user_id>` for personal keys). A `client_credentials` login needs device details on 1.36.
- Ephemeral resources (`ephemeral_*.go`) get the client through `EphemeralResourceData`. Tests check their values with the `echo` provider (`testAccEchoProviderFactories`), which copies them into state, so check that a secret stays out of state in a step without it.
- Vaultwarden 1.34+ derives a member's `access_all` from the role, and 1.37.4 no longer returns it: use `OrganizationUserDetails.HasAccessAll()`. Managers are returned as the custom role (type 4).

**Resource pattern** (all files in `internal/provider/`):
- Each resource has a struct holding the `*vaultwarden.Client` and a model struct with `tfsdk` tags (e.g. `OrganizationUserModel`)
- Implements `resource.Resource`, `resource.ResourceWithConfigure` and usually `resource.ResourceWithImportState`
- Methods: `Metadata()`, `Schema()`, `Configure()`, `Create()`, `Read()`, `Update()`, `Delete()`, `ImportState()`
- Type name format: `vaultwarden_<name>` (set in `Metadata()`)
- `Read()` removes the resource from state when the client returns `ErrNotFound`; deletes of already-deleted objects succeed
- Errors use `resp.Diagnostics.AddError()` with title + detail

**Composite IDs:** Organization-scoped resources import with slash-delimited IDs: `<org_id>/<id>` (`organization_collection`, `organization_group`, `organization_user`), `<org_id>/<collection_id>/<organization_user_id>` (`organization_collection_user`), `<org_id>/<collection_id>/<group_id>` (`organization_collection_group`) and `<org_id>/<group_id>/<organization_user_id>` (`organization_group_member`).

**Resource identity** (`identity.go`): `organization_collection`, `organization_group` and `organization_user` implement `resource.ResourceWithIdentity` with the identity `organization_id` + `id` (the parts of their import ID), and `user` with `id`. Such a resource must set its identity in `Create`, `Read` and `Update` (`setOrganizationObjectIdentity`, `setUserIdentity`): the framework fails the operation when it is missing, and `Read` fills it in for state written before the resource had one. The identity must never change for an existing object. `ImportState` accepts both the import ID and the identity (`importOrganizationObject`). Changing an identity schema needs an identity upgrader and an upgrade test from the last release (`ExternalProviders`), like `TestAccResourceIdentityUpgradeFromV060`.

**List resources** (`list_resource*.go`, for `terraform query`, Terraform 1.14+): `<Name>ListResource` types registered in `ListResources()`, with the same type name as their managed resource, which must implement resource identity. `List()` reuses the client's list methods, filters with the optional `*_contains` attributes (`containsFold`), sorts the objects, and emits each one with `newListResult` (identity, display name, and the resource state when `include_resource` is set, built like the resource's `Read` would). Errors are emitted with `list.ListResultsStreamDiagnostics`. Test them with `Query: true` steps and `querycheck` (`list_resource_test.go`); list blocks in a query step can reference resources created by an earlier step.

## Test Patterns

Acceptance tests require a real Vaultwarden instance. Tests follow this structure:
- `TestAcc<Resource>(t)` — CRUD + import in sequential steps
- `testAcc<Resource>Config(...)` — generates HCL including the provider block, using `test.TestBaseURL`, `test.TestEmail`, `test.TestPassword` and `test.TestAdminToken`
- `testAccPreCheck(t)` logs the shared test user in
- `testAccClient(ctx, t)` (`resource_lifecycle_test.go`) returns the shared test client, to change or check objects outside Terraform in `PreConfig` and check functions
- Uses `resource.TestCheckResourceAttr` and `plancheck.ExpectResourceAction` for assertions
- Provider factory: `testAccProtoV6ProviderFactories`

A regression test must fail without its fix: when two resources change in the same step, Terraform applies them in parallel, which can mask a bug. Change them in separate steps.

## CI

GitHub Actions workflows in `.github/workflows/` (`main.yml` orchestrates `generate`, `lint`, `build`, `test`, and a single `CI gate` job used as the required status check):
- Acceptance tests run a matrix of Vaultwarden {1.36.0, 1.37.4} (the two newest minors; image tags come from `VW_VERSION`, which defaults in `docker-compose.yml` to the newest) × Terraform {1.16.\*, 1.17.\*} (the two non-EOL minors; HashiCorp supports the two newest)
- All `setup-go` steps use `go-version-file: go.mod`, and all actions are pinned to commit SHAs
- `make generate` must produce no git diff, and `go.mod`/`go.sum` must be tidy
- golangci-lint enforced with zero tolerance (`max-issues-per-linter: 0`)
- Also: CodeQL, govulncheck and dependency review (`security.yml`), OpenSSF Scorecard, and actionlint plus zizmor for workflow changes
