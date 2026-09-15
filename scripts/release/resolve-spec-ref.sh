#!/usr/bin/env bash
# Usage: resolve-spec-ref.sh <app-release-tag>
# Echoes the tag if it is a release tag, else refuses: the sync must never fall
# back to a branch or guess the latest release.
set -euo pipefail
ref=${1:-}
if [[ ! "$ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "refusing to sync from '$ref': not an app release tag" >&2
  exit 2
fi
echo "$ref"
