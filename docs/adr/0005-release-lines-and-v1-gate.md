# 5. Release lines and the v1.0.0 gate

Date: 2026-11-25

## Status

Accepted. Replaces the v1.0.0 gate of [ADR 0001](0001-provider-versioning-decoupled-from-api.md); the rest of ADR 0001 stands.

## Context

ADR 0001 kept the provider at 0.x until the managed Keycloak API had a first non-beta version. The provider now also covers the Platform, and v1.0.0 is the release that says it covers both. Waiting for the managed Keycloak API to leave beta would hold that release back for a reason that has nothing to do with the Platform resources.

The managed Keycloak API is still beta, so the provider has to say what a 1.x version promises on top of it.

## Decision

- v1.0.0 is no longer gated on the API's first non-beta date. It is published by a manually run workflow on `main` (`release-v1.yml`) and by nothing else.
- Until a `v1.0.0` tag exists, `main` publishes no release. After it, `main` releases 1.x.
- The 0.x line lives on the `release-0.x` branch. It carries managed Keycloak only and takes fixes and spec syncs for the managed Keycloak API. After v1.0.0 it gets no further releases.
- While the managed Keycloak API is beta, 1.x promises:
  - every managed Keycloak resource, data source, action and attribute of the last 0.x release keeps its name, type and behaviour, and a configuration that plans cleanly on 0.x plans with no changes on 1.0.0;
  - API changes are absorbed by the provider, through the facade and optional attributes, and are not passed on as breaking changes;
  - removals, renames and state-breaking changes wait for 2.0.0.
- The `api_version` rules of ADR 0001 are unchanged: the default is generated from the committed spec, a different pin warns, and there is no client-side enum validation.

## Consequences

- The v1.0.0 date is the owner's call and is no longer tied to an API announcement.
- If the beta API changes shape in a way the provider cannot absorb without breaking a managed Keycloak configuration, the fix is a 2.0.0 release, not a quiet change in 1.x.
- Fixes for managed Keycloak users need no 1.x upgrade before v1.0.0: they ship from `release-0.x`.
