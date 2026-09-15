#!/usr/bin/env bash
# Usage: git log --format='%B%x00' <last-tag>..HEAD | next-version.sh <last-tag>
set -euo pipefail
tag=${1:-}
if [[ ! "$tag" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "last tag '$tag' is not vX.Y.Z; refusing to guess a version" >&2
  exit 2
fi
major=${BASH_REMATCH[1]} minor=${BASH_REMATCH[2]} patch=${BASH_REMATCH[3]}

bump="patch"
seen=0
while IFS= read -r -d '' msg; do
  msg=${msg#$'\n'}
  [[ -z "$msg" ]] && continue
  seen=1
  # Every line, not just the subject: a squash merge lists the PR's own commits
  # in its body as "* type: ...", and a breaking footer sits on its own line.
  while IFS= read -r line; do
    line=${line#\* }
    if [[ "$line" =~ ^[a-z]+(\([^\)]*\))?!: || "$line" =~ ^BREAKING[\ -]CHANGE: ]]; then
      echo "breaking change needs a human release: ${msg%%$'\n'*}" >&2
      exit 3
    fi
    if [[ "$line" =~ ^feat(\([^\)]*\))?: ]]; then
      bump="minor"
    fi
  done <<< "$msg"
done

if [[ $seen -eq 0 ]]; then
  echo "no commits since $tag; nothing to release" >&2
  exit 4
fi

if [[ $bump == minor ]]; then
  echo "v$major.$((minor + 1)).0"
else
  echo "v$major.$minor.$((patch + 1))"
fi
