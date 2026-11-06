#!/usr/bin/env bash
# Usage: git log --format='%B%x00' <last-tag>..HEAD | should-release.sh
# Exits 0 when a commit could change what users get. It reads a commit as
# next-version.sh does: the subject, a merge commit's PR title, the "* type: ..."
# lines of a squash, and a breaking footer. Only titles that clearly cannot change
# anything are passed over, so a squash under a loose title still releases.
set -euo pipefail

matters() {
  local line=$1
  # A breaking mark always goes on, so next-version.sh can stop for a person.
  [[ "$line" =~ ^[a-z]+(\([^\)]*\))?!: ]] && return 0
  [[ "$line" =~ ^chore(\([^\)]*\))?:\ sync\ OpenAPI\ spec ]] && return 0
  [[ "$line" =~ ^(docs|test|tests|ci|build|style|chore)(\([^\)]*\))?: ]] && return 1
  return 0
}

while IFS= read -r -d '' msg; do
  msg=${msg#$'\n'}
  [[ -z "$msg" ]] && continue
  first=1
  title_next=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" =~ ^BREAKING[\ -]CHANGE: ]]; then
      exit 0
    fi
    if [[ $first -eq 1 ]]; then
      first=0
      if [[ "$line" == "Merge pull request "* ]]; then
        title_next=1
      elif matters "$line"; then
        exit 0
      fi
    elif [[ $title_next -eq 1 ]]; then
      if [[ -n "$line" ]]; then
        title_next=0
        if matters "$line"; then
          exit 0
        fi
      fi
    elif [[ "$line" == "* "* ]]; then
      if matters "${line#\* }"; then
        exit 0
      fi
    fi
  done <<< "$msg"
done
exit 1
