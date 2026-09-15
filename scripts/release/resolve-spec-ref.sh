#!/usr/bin/env bash
# Usage: resolve-spec-ref.sh <dispatched-ref> <latest-app-release>
set -euo pipefail
ref=${1:-$2}
if [[ ! "$ref" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "refusing to sync from '$ref': not an app release tag" >&2
  exit 2
fi
echo "$ref"
