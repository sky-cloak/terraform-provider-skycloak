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
  # A type counts on the subject and on "* type: ..." lines, which is how a squash
  # merge lists the PR's own commits; other body lines are prose. A breaking
  # footer counts on any line.
  first=1
  while IFS= read -r line; do
    if [[ "$line" =~ ^BREAKING[\ -]CHANGE: ]]; then
      echo "breaking change needs a human release: ${msg%%$'\n'*}" >&2
      exit 3
    fi
    if [[ $first -eq 1 ]]; then
      first=0
    elif [[ "$line" == "* "* ]]; then
      line=${line#\* }
    else
      continue
    fi
    if [[ "$line" =~ ^[a-z]+(\([^\)]*\))?!: ]]; then
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
