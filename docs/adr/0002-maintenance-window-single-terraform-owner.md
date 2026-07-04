# 2. The maintenance window has a single Terraform owner

Date: 2026-07-04

## Status

Accepted

## Context

The API exposes a cluster's maintenance window twice: embedded in the cluster create body (one-call convenience for programmatic callers) and as a dedicated `/clusters/{id}/maintenance-window` sub-resource. Mirroring both into Terraform would let two resources manage the same attribute, which produces permanent-diff fights (the well-known inline-vs-standalone rules problem in other providers).

## Decision

`skycloak_cluster_maintenance_window` is the **only** Terraform owner of the window. The embedded `maintenance_window` field in the cluster create body is deliberately not mapped on `skycloak_cluster`. Terraform users create the cluster and the window as two resources in one apply, which is idiomatic.

`terraform destroy` on the window resource calls the API delete, which reverts the cluster to the **workspace default window**; a managed cluster always retains an effective window (a region default is provisioned when nothing else applies).

## Consequences

- No dual-ownership drift is possible.
- A cluster created without a window resource silently receives the platform's region-default window, and `skycloak_cluster` does not display it; users add the dedicated resource when they want control.
- One-call create parity is an API-level convenience Terraform intentionally does not use.
