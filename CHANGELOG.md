# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Provider configuration: `endpoint`, `api_key` (sensitive), `api_version`, with environment-variable fallbacks.
- Resources: `skycloak_cluster` (async create + import), `skycloak_realm`, `skycloak_application`, `skycloak_smtp`.

### Changed
- Resources and data sources now call the generated API client directly, so they stay in sync with the OpenAPI spec on `make generate`.
- `skycloak_application`: `type` is now immutable (changing it replaces the resource) and `service_account_enabled` is read-only, matching the API contract.

### Removed
- `skycloak_identity_provider` is temporarily withdrawn pending a redesign of its `config` to a structured form (OIDC/LDAP/SAML blocks) that matches the API; it will return in a subsequent release.
- Data sources: `skycloak_cluster`, `skycloak_cluster_locations`, `skycloak_cluster_types`, `skycloak_cluster_features`.
- Typed API client generated from the Skycloak OpenAPI specification (oapi-codegen) with a `make generate` workflow.
- Generated documentation, unit tests, an acceptance-test harness, CI, and a signed release pipeline.

[Unreleased]: https://github.com/sky-cloak/terraform-provider-skycloak/commits/main
