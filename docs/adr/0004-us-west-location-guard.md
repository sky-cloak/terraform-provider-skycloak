# 4. US West location guard

Date: 2026-09-25

## Status

Accepted

## Context

The public API is adding `us-west` (US West) as its own cluster location. Until
now it reported every US cluster as `us`, East or West, so a US West cluster
imported into Terraform sits in state, and usually in configuration, as `us`.
Creating a `us-west` cluster was rejected, so importing is the only way such a
cluster got into state.

`location` is `Required` and used to be `RequiresReplace`. The provider copied
the API value into state on every read. Once the API reports `us-west` for that
cluster, the refreshed state says `us-west`, the configuration says `us`, and
Terraform plans a destroy and recreate of a cluster that never moved.

A plan modifier cannot hide the difference: Terraform core requires the planned
value of a non-computed attribute to equal the configuration, so the provider
cannot plan `us-west` when the configuration says `us`.

## Decision

The guard has three parts, in `internal/provider/cluster_location.go`.

- **Read keeps `us`.** When the prior state says `us` and the API reports
  `us-west`, state keeps `us` and Read emits a warning telling the user to set
  `location = "us-west"`. Every other reported value is stored as reported.
  Create and Update apply the same rule to the planned value, so an unrelated
  update (a size change, say) never writes `us-west` over a planned `us`, which
  Terraform would reject as an inconsistent result.
- **The reported location is kept in private state** under `api_location`,
  written by Create, Read and Update. That is the only way plan-time logic can
  tell a US West cluster recorded as `us` from a real US East cluster, because
  both look the same in state and configuration.
- **`RequiresReplace` becomes `RequiresReplaceIf`.** A location change replaces
  the cluster unless both the old and new values are `us` or `us-west` and the
  recorded API location is `us-west`. That relabel is an in-place update, and
  Update then reads the cluster instead of patching it, so no call that could
  change the cluster is made. An unknown planned value, or no recorded API
  location, is treated as a move.

## Consequences

- An existing configuration saying `us` for a US West cluster plans no change
  and prints the warning. Changing it to `us-west` applies in place.
- Importing a US West cluster plans no replacement whether the configuration
  says `us` (an in-place relabel back to `us`, then the warning) or `us-west`
  (no change).
- Real moves still replace: a US East cluster changed to `us-west`, and any
  change between the US codes and `ca`, `eu` or `au`.
- Against an API that still reports US West clusters as `us`, the guard is
  dormant: the recorded location is `us`, so changing such a cluster's
  configuration to `us-west` still plans a replacement. Users should change the
  configuration only once the warning appears.
- Provider releases without the guard do plan a replacement once the API
  reports `us-west`, so users managing US West clusters have to upgrade before
  that happens.
- `plan -refresh=false` on a state written by an older provider has no recorded
  API location, so a relabel plans a replacement there. A normal plan refreshes
  first and records it.
