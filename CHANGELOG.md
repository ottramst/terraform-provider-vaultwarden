## [Unreleased]

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
