# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres
to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.5.0] - 2026-09-04

### Added
- `skycloak_cluster_versions`: new `version_details` attribute exposing, per version, whether it is still `active` (offered for new clusters and upgrades), whether it `is_major_change`, and its `breaking_change_count`.

### Changed
- The cluster-type versions endpoint now returns structured objects instead of bare strings. `versions` keeps its existing list-of-strings shape and ordering, so no configuration or state change is required.

## [0.4.0] - 2026-07-27

### Added
- `skycloak_realm_export`: exports a single realm (configuration, users, and credentials) as an encrypted archive and waits for the job to finish. The archive expires 24 hours after completion.
- `skycloak_realm_import`: imports a realm into a cluster, either by uploading a local artifact through a presigned URL (`source_kind = "upload"`) or from a stored realm export (`source_kind = "stored"`).

## [0.3.4] - 2026-07-09

### Changed
- Provider overview docs sharpened for discovery and AI citability: a "what you can manage" summary, keyword coverage (OIDC, SAML, SSO, WAF), a concrete cluster example, and links to skycloak.io.

## [0.3.3] - 2026-07-08

### Fixed
- Retry `409` responses that carry a `Retry-After` header, so transient cluster-updating conflicts are retried instead of surfaced as errors.

## [0.3.2] - 2026-07-05

### Fixed
- `skycloak_domain`: creating a domain crashed with a "Value Conversion Error" on `dns_records`. The computed `dns_records` list is now modeled as a framework `types.List` so it can carry its unknown-at-plan value.
- `skycloak_custom_theme`: uploading a theme now waits until it finishes deploying before returning, so a same-apply `skycloak_theme_assignment` no longer races the async rollout and fails with "Theme is not deployed".
- `skycloak_custom_theme`: deleting a theme now waits for the asynchronous undeploy to finish, so a same-apply content replace no longer hits a "theme name conflicts with existing theme" error while the old copy is still being removed.

## [0.3.1] - 2026-07-04

### Fixed
- `skycloak_siem_destination`: an omitted `batch` block caused "Provider produced inconsistent result after apply". `batch` and its fields are now Optional+Computed with the API's defaults (`max_events` 1000, `max_interval_seconds` 60).
- `skycloak_webhook_subscription`: documented that `signing_secret` must be 32 to 512 characters (the API rejects shorter values).
- `skycloak_captcha_domain`: documented that `hostname` must be one of the cluster's verified custom domains.

## [0.3.0] - 2026-07-03

### Added
- `skycloak_siem_destination` resource: SIEM forwarding to syslog, S3, or HTTP collectors, with source/batch tuning and write-only credentials. Plus `skycloak_siem_destination` and `skycloak_siem_destinations` data sources.
- `skycloak_webhook_subscription` resource: webhook deliveries with per-subscription event types, optional cluster/realm scoping, and write-only `signing_secret`/`authorization_header`. Plus `skycloak_webhook_subscriptions` and `skycloak_webhook_event_types` data sources.
- `skycloak_cluster_maintenance_window` resource: the cluster's own upgrade/maintenance window. Destroy reverts the cluster to the workspace default window.
- `skycloak_captcha_domain` resource: hostnames registered for CAPTCHA protection.
- `captcha` block on `skycloak_cluster_security` (`enabled`, `enabled_realms`); omitted blocks keep the server value untouched, as before.
- `auto_upgrade_enabled` on the `skycloak_cluster` resource and data source.
- Terraform **actions** (require Terraform >= 1.14): `skycloak_test_smtp`, `skycloak_test_identity_provider`, `skycloak_cancel_cluster_upgrade`, `skycloak_test_siem_destination`, `skycloak_test_webhook_subscription`.
- With these, the provider covers 100% of the public API's generated operations (`scripts/check-api-coverage.sh`).

### Fixed
- Authentication header renamed from `apikey` to `API-Key`, matching the API's new auth header (requests were failing with 401).

### Changed
- `skycloak_theme` is renamed to `skycloak_custom_theme` (symmetry with `skycloak_custom_extension`). State must be re-imported under the new type; `moved` blocks cannot cross resource types.
- The unpinned default `api_version` is now generated from the committed OpenAPI spec (`tools/gen-version`), and pinning a different version emits a warning diagnostic.
- Every API call now sends the pinned `API-Version` through the generated per-operation parameters (the API made the header a required parameter).
- An unpinned `api_version` now defaults to the version the provider was built against instead of omitting the header, since the API requires it.
- `skycloak_cluster_credentials` data source now returns the cluster automation service account (`client_id`, `client_secret`, `token_url`) instead of admin console credentials, following the API change.

### Removed
- `skycloak_cluster_builds` and `skycloak_cluster_build` data sources: the API removed the cluster build endpoints.

## [0.2.0] - 2026-05-26

### Added
- `skycloak_domain` and `skycloak_domain_route` resources (custom domains + realm routing; `dns_records` surfaced for verification).
- `skycloak_login_branding` and `skycloak_email_branding` resources (per-realm branding: colors, logos, footer, plus nested `internationalization` and `sso` blocks).
- `skycloak_theme_assignment` (realm-level theme per Keycloak theme type) and `skycloak_client_theme_assignment` (per-client login theme) resources.
- `skycloak_themes` data source listing custom themes uploaded to a cluster.
- `skycloak_cluster_extension` resource (install a marketplace extension on a cluster, with parameters) and `skycloak_extensions` data source (extension catalog).
- `skycloak_export` resource — starts a database export and waits for completion, surfacing the presigned `download_url`, checksum, and size.
- `skycloak_theme` resource — uploads a custom theme archive (ZIP/Keycloakify JAR); detects file-content changes via a computed `content_sha256`.
- `skycloak_custom_extension` resource — uploads a custom extension JAR with its parameter schema; a JAR/version change publishes a new version, metadata updates in place.
- Realm RBAC: `skycloak_realm_role`, `skycloak_realm_group`, `skycloak_realm_user` resources, plus `skycloak_realm_user_role_assignment` and `skycloak_realm_group_membership` edge resources; `skycloak_realm_roles`, `skycloak_realm_groups`, and `skycloak_realm_users` data sources.
- `skycloak_application_role_assignment` resource (grant a role to an application's service account); `skycloak_application_roles` and `skycloak_application_sessions` data sources.
- `skycloak_cluster_security` resource — edge security: per-path IP allow-listing, rate limiting, WAF (presets, categories, exclusions), geo-blocking, and bot management. CAPTCHA settings are left untouched.
- Read-only data sources: `skycloak_cluster_versions`, `skycloak_identity_provider_templates`, `skycloak_domain_routes`, `skycloak_cluster_builds`, `skycloak_cluster_upgrades`, `skycloak_oidc_discovery`, `skycloak_cluster_insights`, `skycloak_cluster_credentials`, `skycloak_realm_group_members`, `skycloak_cluster_upgrade_path`, `skycloak_cluster_logs`, `skycloak_cluster_security_logs`, `skycloak_cluster_events`, `skycloak_cluster_build`, `skycloak_cluster_events_export`.

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

[0.3.4]: https://github.com/sky-cloak/terraform-provider-skycloak/compare/v0.3.3...v0.3.4
[0.3.3]: https://github.com/sky-cloak/terraform-provider-skycloak/compare/v0.3.2...v0.3.3
[0.3.2]: https://github.com/sky-cloak/terraform-provider-skycloak/compare/v0.3.1...v0.3.2
[0.3.1]: https://github.com/sky-cloak/terraform-provider-skycloak/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/sky-cloak/terraform-provider-skycloak/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/sky-cloak/terraform-provider-skycloak/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/sky-cloak/terraform-provider-skycloak/releases/tag/v0.1.0
