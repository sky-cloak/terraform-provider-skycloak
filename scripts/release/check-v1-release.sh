#!/usr/bin/env bash
# Usage: check-v1-release.sh <version> <commit>
# Refuses (exit 1) unless <version> can be published from <commit>: the tag does
# not exist yet, the test workflow has a successful run for the commit, and the
# changelog has an entry for the version. Needs GH_TOKEN for gh.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
version=${1:-}
sha=${2:-}
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ || -z "$sha" ]]; then
  echo "usage: check-v1-release.sh <vX.Y.Z> <commit>" >&2
  exit 64
fi

exists=$("$here/tag-exists.sh" "$version")
if [[ "$exists" == yes ]]; then
  echo "$version already exists; a published release is never retagged" >&2
  exit 1
fi

# Published only from a commit whose tests passed.
passed=$("${GH_BIN:-gh}" run list --workflow test --commit "$sha" --status success --json databaseId --jq 'length')
if [[ "$passed" -lt 1 ]]; then
  echo "the test workflow has no successful run for $sha" >&2
  exit 1
fi

# The changelog entry is the release's record, so it must be there.
heading="## [${version#v}]"
if ! grep -qF -- "$heading" "${CHANGELOG:-CHANGELOG.md}"; then
  echo "the changelog has no ${version#v} entry" >&2
  exit 1
fi
