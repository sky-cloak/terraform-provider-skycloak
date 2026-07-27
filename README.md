# terraform-provider-skycloak

Official [Terraform](https://www.terraform.io) provider for **Skycloak**: manage your managed-Keycloak platform as code, from clusters, edge security, and custom domains to realms, applications, and more.

> **Status:** early release. Resource coverage is growing; see the changelog for what's available.

## Requirements

- Terraform >= 1.5
- A Skycloak account and an API key (create one in the Skycloak dashboard)

## Usage

```hcl
terraform {
  required_providers {
    skycloak = {
      source  = "sky-cloak/skycloak"
      version = "~> 0.1"
    }
  }
}

provider "skycloak" {
  # api_key is read from SKYCLOAK_API_KEY (recommended)
  # endpoint defaults to https://api.skycloak.io
}

resource "skycloak_cluster" "production" {
  name     = "production"
  type     = "keycloak"
  size     = "small"
  version  = "26.1"
  location = "eu"
}
```

## Authentication

The provider authenticates with a Skycloak API key. Create one in the
[Skycloak dashboard](https://app.skycloak.io) and set it via the `SKYCLOAK_API_KEY`
environment variable (preferred) or the `api_key` provider argument.

Requests are rate limited according to your Skycloak plan. On a `429` response,
honor the `Retry-After` header.

| Argument | Env var | Default |
|---|---|---|
| `api_key` | `SKYCLOAK_API_KEY` | — (required) |
| `endpoint` | `SKYCLOAK_ENDPOINT` | `https://api.skycloak.io` |
| `api_version` | `SKYCLOAK_API_VERSION` | current API version |

## Resources & data sources

| Resource | Description |
|---|---|
| `skycloak_cluster` | A managed Keycloak cluster (incl. `auto_upgrade_enabled`) |
| `skycloak_cluster_maintenance_window` | The cluster's upgrade/maintenance window |
| `skycloak_cluster_security` | Edge security: IP allow-listing, rate limiting, WAF, geo-blocking, bot management, CAPTCHA |
| `skycloak_captcha_domain` | A hostname registered for CAPTCHA protection |
| `skycloak_domain` | A custom domain on a cluster |
| `skycloak_domain_route` | Maps a realm onto a custom domain |
| `skycloak_realm` | A realm within a cluster |
| `skycloak_realm_user` | A realm user |
| `skycloak_realm_role` | A realm-scoped role |
| `skycloak_realm_group` | A realm group (optionally nested) |
| `skycloak_realm_user_role_assignment` | Assigns a realm role to a user |
| `skycloak_realm_group_membership` | Adds a user to a realm group |
| `skycloak_application` | An OIDC/SAML client |
| `skycloak_application_secret` | Rotates an application's client secret |
| `skycloak_application_role_assignment` | Grants a role to an application's service account |
| `skycloak_identity_provider` | An SSO connection (OIDC/LDAP/SAML) |
| `skycloak_login_branding` | Login-page branding for a realm |
| `skycloak_email_branding` | Email-template branding for a realm |
| `skycloak_custom_theme` | Uploads a custom theme archive to a cluster |
| `skycloak_theme_assignment` | Realm-level custom theme per Keycloak theme type |
| `skycloak_client_theme_assignment` | Per-client login-theme override |
| `skycloak_cluster_extension` | A marketplace extension installed on a cluster |
| `skycloak_custom_extension` | Uploads a custom extension JAR to the workspace catalog |
| `skycloak_smtp` | Realm SMTP configuration |
| `skycloak_export` | A database export job (waits for completion) |
| `skycloak_realm_export` | An encrypted single-realm export (waits for completion) |
| `skycloak_realm_import` | Imports a realm from a local artifact or a stored export |
| `skycloak_siem_destination` | SIEM forwarding to syslog, S3, or HTTP collectors |
| `skycloak_webhook_subscription` | Webhook deliveries for workspace and Keycloak events |

| Data source | Description |
|---|---|
| `skycloak_cluster` | Look up a cluster by ID |
| `skycloak_cluster_locations` | Supported regions |
| `skycloak_cluster_types` | Supported cluster types |
| `skycloak_cluster_versions` | Keycloak versions for a cluster type |
| `skycloak_cluster_features` | Available Keycloak feature flags |
| `skycloak_cluster_upgrades` | Version-upgrade history for a cluster |
| `skycloak_cluster_upgrade_path` | Recommended version-upgrade path |
| `skycloak_cluster_credentials` | A cluster's automation service account credentials |
| `skycloak_cluster_insights` | Cluster analytics (overview/auth/events/performance/security) as JSON |
| `skycloak_cluster_logs` | Recent application logs |
| `skycloak_cluster_security_logs` | Recent edge-security logs (WAF, geo, rate limiting) |
| `skycloak_cluster_events` | Recent Keycloak admin/user events |
| `skycloak_cluster_events_export` | Exported events document |
| `skycloak_domain_routes` | Realm routes on a custom domain |
| `skycloak_realm_users` | Users in a realm |
| `skycloak_realm_roles` | Realm-scoped roles in a realm |
| `skycloak_realm_groups` | Top-level groups in a realm |
| `skycloak_realm_group_members` | Users in a realm group |
| `skycloak_application_roles` | Roles on an application's service account |
| `skycloak_application_sessions` | Active sessions for an application |
| `skycloak_identity_provider_templates` | Identity-provider template catalog |
| `skycloak_oidc_discovery` | Resolve an OIDC issuer's endpoints |
| `skycloak_themes` | Custom themes uploaded to a cluster |
| `skycloak_extensions` | Extension catalog available to the workspace |
| `skycloak_siem_destination` | Look up a SIEM destination and its delivery health |
| `skycloak_siem_destinations` | All SIEM destinations in the workspace |
| `skycloak_webhook_subscriptions` | All webhook subscriptions in the workspace |
| `skycloak_webhook_event_types` | Webhook event type catalog |

| Action (Terraform >= 1.14) | Description |
|---|---|
| `skycloak_test_smtp` | Sends a probe email through a realm's SMTP configuration |
| `skycloak_test_identity_provider` | Validates an identity provider's connection |
| `skycloak_cancel_cluster_upgrade` | Cancels a scheduled or in-progress upgrade |
| `skycloak_test_siem_destination` | Sends a synthetic event through a SIEM destination |
| `skycloak_test_webhook_subscription` | Delivers a sample event to a webhook endpoint |

Full reference docs are generated under [`docs/`](./docs) and published on the Terraform Registry.

## Development

```bash
make build      # build the provider
make test       # unit tests
make testacc    # acceptance tests (creates real resources; needs SKYCLOAK_API_KEY)
make lint       # golangci-lint
make docs       # regenerate docs from schema + examples
make generate   # regenerate the API client from the OpenAPI spec
```

The API client under `internal/apiclient` is generated from the Skycloak OpenAPI
specification with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen);
run `make generate` after updating `internal/apiclient/openapi.yaml`.

## Keeping in sync with the API

The client in `internal/apiclient` is generated from `internal/apiclient/openapi.yaml` with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) — run `make generate` to refresh it. CI fails if the committed generated code drifts from the spec. Requests are retried on `429`/`5xx` responses with `Retry-After`-aware backoff.

## Contributing

See [TESTING.md](./TESTING.md) for the test and release workflow. Every change ships with tests and a changelog entry.

## License

[MPL-2.0](./LICENSE).
