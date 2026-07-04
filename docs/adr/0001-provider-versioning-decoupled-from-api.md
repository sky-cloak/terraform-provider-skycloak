# 1. Provider versioning is decoupled from the calendar-versioned API

Date: 2026-07-04

## Status

Accepted

## Context

The Skycloak public API is calendar-versioned (`API-Version` header, e.g. `2026-06-01.beta`) and requires the header on every request. The Terraform Registry requires semver for providers. The two schemes cannot be unified: the API bumps its date rarely while the provider needs frequent patch and minor releases, and a date is not a semver.

The API's only published version currently carries a `.beta` suffix, meaning its shape may still change.

## Decision

- The provider follows its own semver, independent of the API date.
- **v1.0.0 is gated on the API's first GA (non-beta) date.** While the API is beta, the provider stays 0.x, because a 1.x stability promise on top of a beta API shape is not honest.
- The unpinned default `api_version` is **generated from the committed OpenAPI spec** (`tools/gen-version` writes `DefaultAPIVersion` from the spec's `Versions` enum during `make generate`), so the default always matches the client the provider was generated against.
- A user may pin any `api_version`; a pin different from the built-against version produces a **warning** diagnostic, never an error, and there is **no client-side enum validation**, so old provider binaries keep working when the server adds new dates.

## Consequences

- Cutting v1.0.0 is triggered by an API announcement, not a feature milestone.
- Spec syncs bump the default version and regenerate the client in the same commit; CI proves the facade still compiles against it.
- Users who pin forward take responsibility for unmapped response shapes; the warning makes that visible.
