#!/usr/bin/env bash
# Usage: check-release-tag.sh <tag> [<pushing actor>]
# Run in a full clone. Refuses (exit 1) a release tag that is on the wrong line:
#   v0.*  must be on release-0.x, and is refused once v1.0.0 exists
#   v1.0.0 must be pushed by the release bot (the release-v1 workflow), on main
#   v2+ must be on main
#   v1.*  other than v1.0.0 needs the v1.0.0 tag first, and must be on main
set -euo pipefail
tag=${1:-}
if [[ ! "$tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "refusing $tag: not a release tag" >&2
  exit 2
fi
major=${BASH_REMATCH[1]}
actor=${2:-}
bot="skycloak-releaser-bot[bot]"
git=${GIT_BIN:-git}

on_branch() {
  "$git" merge-base --is-ancestor "refs/tags/$tag" "refs/remotes/origin/$1" 2>/dev/null
}

if [[ "$major" == 0 ]]; then
  if "$git" rev-parse -q --verify "refs/tags/v1.0.0" >/dev/null; then
    echo "refusing $tag: v1.0.0 exists, so release-0.x gets no further releases" >&2
    exit 1
  fi
  if ! on_branch release-0.x; then
    echo "refusing $tag: its commit is not on release-0.x" >&2
    exit 1
  fi
  exit 0
fi

if [[ "$tag" == v1.0.0 && "$actor" != "$bot" ]]; then
  echo "refusing $tag: only the release-v1 workflow publishes it" >&2
  exit 1
fi
if [[ "$tag" != v1.0.0 && "$major" == 1 ]]; then
  if ! "$git" rev-parse -q --verify "refs/tags/v1.0.0" >/dev/null; then
    echo "refusing $tag: v1.0.0 does not exist yet" >&2
    exit 1
  fi
fi
if ! on_branch main; then
  echo "refusing $tag: its commit is not on main" >&2
  exit 1
fi
