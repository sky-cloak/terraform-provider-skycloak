# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `skycloak_domain` and `skycloak_domain_route` resources (custom domains + realm routing; `dns_records` surfaced for verification).

## [0.1.0] - 2026-05-25

### Added
- Provider configuration: `endpoint`, `api_key` (sensitive), `api_version`, with environment-variable fallbacks.
- Resources: `skycloak_cluster` (async create + import), `skycloak_realm`, `skycloak_application`, `skycloak_identity_provider`, `skycloak_smtp`.
- Data sources: `skycloak_cluster`, `skycloak_cluster_locations`, `skycloak_cluster_types`, `skycloak_cluster_features`.
- Typed API client generated from the Skycloak OpenAPI specification (oapi-codegen) with a `make generate` workflow.
- Generated documentation, unit tests, an acceptance-test harness, CI, and a signed release pipeline.
- Automatic `Retry-After`-aware retries on `429`/`5xx` responses.
- `spec-sync` workflow + `scripts/check-api-coverage.sh` that detect upstream OpenAPI drift and report API operations not yet exposed.
- `skycloak_application_secret` resource that rotates an application's client secret (regenerates on create or when `rotate_when` changes).
- `skycloak_application` list reads now follow pagination across all pages.

### Changed
- Resources and data sources call the generated API client directly, so they stay in sync with the OpenAPI spec on `make generate`.
- `skycloak_application`: `type` is immutable (changing it replaces the resource) and `service_account_enabled` is read-only, matching the API contract.
- `skycloak_identity_provider`: `config` is a structured block (`oidc` / `ldap` / `saml` sub-objects plus `attribute_mappings`, `button_text`, `icon_url`, `sync_mode`, `trust_email`), matching the API contract.

[Unreleased]: https://github.com/sky-cloak/terraform-provider-skycloak/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/sky-cloak/terraform-provider-skycloak/releases/tag/v0.1.0
