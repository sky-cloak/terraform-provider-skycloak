#!/usr/bin/env bash
# Usage: git log --format=%s <last-tag>..HEAD | should-release.sh
# Exits 0 when a subject could change what users get. Only subjects that clearly
# cannot are passed over, so a squash merge under a loose title still releases.
set -euo pipefail
while IFS= read -r subject || [[ -n "$subject" ]]; do
  [[ -z "$subject" ]] && continue
  if [[ "$subject" =~ ^chore(\([^\)]*\))?:\ sync\ OpenAPI\ spec ]]; then
    exit 0
  fi
  # A breaking mark always goes on, so next-version.sh can stop for a person.
  if [[ "$subject" =~ ^[a-z]+(\([^\)]*\))?!: ]]; then
    exit 0
  fi
  if [[ "$subject" =~ ^(docs|test|tests|ci|build|style|chore)(\([^\)]*\))?: ]]; then
    continue
  fi
  exit 0
done
exit 1
