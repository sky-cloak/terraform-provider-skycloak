# terraform-provider-skycloak

Official [Terraform](https://www.terraform.io) provider for **Skycloak** — manage your managed-Keycloak environment (clusters, realms, applications, identity providers, SMTP) as code.

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
| `skycloak_cluster` | A managed Keycloak cluster |
| `skycloak_realm` | A realm within a cluster |
| `skycloak_application` | An OIDC/SAML client |
| `skycloak_smtp` | Realm SMTP configuration |

| Data source | Description |
|---|---|
| `skycloak_cluster` | Look up a cluster by ID |
| `skycloak_cluster_locations` | Supported regions |
| `skycloak_cluster_types` | Supported cluster types |
| `skycloak_cluster_features` | Available Keycloak feature flags |

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

## Contributing

See [TESTING.md](./TESTING.md) for the test and release workflow. Every change ships with tests and a changelog entry.

## License

[MPL-2.0](./LICENSE).
