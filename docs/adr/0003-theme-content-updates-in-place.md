# 3. Custom theme content updates in place

Date: 2026-09-07

## Status

Accepted

## Context

`skycloak_custom_theme` used to mark its content hash `RequiresReplace`: new
archive bytes meant destroy then create. A theme name is unique per cluster, so
the replacement could not be created before the original was deleted, and both
halves are asynchronous (Keycloak undeploys, then extracts and rolls out the new
package). Between them the realms and application clients pointing at the theme
lost their assignment and fell back to Keycloak's built-in look, and the new
theme came back with a new ID, so every `skycloak_theme_assignment` referring to
it changed too.

Terraform users worked around this by uploading the new archive under a
different name with `create_before_destroy`, repointing the realm, and deleting
the old theme by hand. That is a workaround for a modelling error, not for an
API limitation: the public API now exposes
`PUT /clusters/{cluster_id}/themes/{theme_id}/content`, which replaces a theme's
archive while keeping its identity and repointing every realm and application
that uses it.

## Decision

Content is an updatable attribute, not an identity attribute.

- `content_sha256` carries no `RequiresReplace` modifier. A content-only change
  produces an in-place update plan.
- `Update` sends the new archive to the content endpoint and then waits for the
  theme to report `deployed`, the same wait the create path performs, so a
  same-apply assignment never races the rollout.
- Name and description changes still go to `PATCH /themes/{theme_id}`. A new
  version label rides along with the content when both change in one apply,
  because the API records the label only once the new content is live. Removing
  the version is the exception: the content request omits an empty version and
  the API reads that as "keep the current label", so a removal goes through
  `PATCH` (sent as an explicit empty string) before the content request.
- `theme_types` and `cluster_id` keep `RequiresReplace`. The content endpoint
  carries neither, and it rejects an archive that drops a theme type the theme
  currently provides, so those two remain identity for Terraform's purposes.
- When the post-update deploy wait fails, the provider writes the theme returned
  by the content call (including the new content hash) to state before raising
  the error: the archive upstream is already the new one, and state must not
  claim otherwise.

## Consequences

- Editing a theme is no longer disruptive: the ID, the name, and every realm and
  client assignment survive an archive change, so the sign-in page is never left
  unbranded mid-apply.
- No state migration is needed. Removing a plan modifier does not change the
  shape of stored state, so configurations on the rename plus
  `create_before_destroy` workaround keep working; those users can delete the
  `lifecycle` block and the duplicate theme resource at their own pace.
- The provider now needs an API that serves the content endpoint. A workspace
  pinned with `api_version` to a version predating it gets a `404` on a content
  change instead of the old destroy/create behavior.
- A theme created by a platform migration has pinned content and answers `409`.
  That surfaces as an update error, which is the honest outcome: the provider no
  longer silently deletes and re-uploads such a theme.
