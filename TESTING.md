# Testing & versioning

How every change to `terraform-provider-skycloak` is verified and released. **No change merges red.**

## The rule for every change

Every PR that adds or changes behavior MUST include, in the same PR:

1. **Tests**
   - New resource/data source → an **acceptance test** (`resource.Test`, gated by `TF_ACC`) covering create → read → update → import → destroy, plus a `CheckDestroy`.
   - Client change → an `httptest`-backed unit test asserting headers, body, and response/error decoding.
   - Schema change → it is exercised by the provider unit tests (`TestProviderSchema`).
2. **Docs** — regenerate with `make docs` (tfplugindocs) and commit the result; CI fails on drift.
3. **Example** — add/adjust the matching file under `examples/` (these feed the generated docs).
4. **Checklist** — tick/extend [`CHECKLIST.md`](./CHECKLIST.md).
5. **Changelog** — an entry under `## [Unreleased]` in [`CHANGELOG.md`](./CHANGELOG.md).
6. **Green gates** — `make lint test build docs` pass locally; CI re-runs them.

## Test layers

| Layer | What | Command | Gate |
|---|---|---|---|
| Unit | Provider schema/metadata, client (httptest) | `make test` | CI on every push/PR |
| Lint | govet, staticcheck, revive, … | `make lint` | CI |
| Tidy | `go.mod`/`go.sum` tidy & committed | `go mod tidy && git diff --exit-code` | CI |
| Docs | Generated docs match schema/examples | `make docs && git diff --exit-code` | CI |
| Acceptance | Real CRUD against a dev workspace | `make testacc` | manual / scheduled (`workflow_dispatch`) with `SKYCLOAK_API_KEY` |

Acceptance tests create real clusters — run them against a **disposable dev workspace** only. They self-skip unless both `TF_ACC=1` and `SKYCLOAK_API_KEY` are set. Add sweepers before enabling them on a schedule.

## Running locally

```bash
make test                      # unit tests
make lint                      # golangci-lint
make docs                      # regenerate docs/ from schema + examples
TF_ACC=1 SKYCLOAK_API_KEY=sk_sc_... make testacc   # acceptance (creates resources!)
```

To try the provider by hand, build it and point Terraform at the local binary with a `~/.terraformrc` `dev_overrides` block for `sky-cloak/skycloak`.

## Versioning

- **SemVer** (`vMAJOR.MINOR.PATCH`), `0.x` while resources are still being added.
  - PATCH: bug fixes, no schema change. MINOR: new resources/data sources or new optional attributes. MAJOR: removed/renamed attributes or resources, or any state-breaking change.
- **State compatibility:** never remove/rename an attribute without a state-upgrade path; new attributes are `Optional`/`Computed`.
- **API surface** is pinned via the `api_version` provider argument (`API-Version` header), independent of the provider version.
- **Conventional commits** (`feat:`/`fix:`/`docs:`…) drive release notes.
- **Releasing:** move `## [Unreleased]` under a new version heading, then tag:
  ```bash
  git tag v0.1.0 && git push origin v0.1.0
  ```
  The `release` workflow runs `goreleaser`, GPG-signs the checksums, and publishes the GitHub release. The Terraform Registry picks up new signed `vX.Y.Z` tags once the provider is listed.
