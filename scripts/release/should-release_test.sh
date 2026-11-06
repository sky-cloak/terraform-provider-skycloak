#!/usr/bin/env bash
# Run: bash scripts/release/should-release_test.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0

# check NAME WANT_EXIT [COMMIT_MESSAGE...]: 0 releases, 1 does not
check() {
  local name=$1 want=$2
  shift 2
  local code=0
  if [[ $# -gt 0 ]]; then
    printf '%s\0' "$@" | "$here/should-release.sh" || code=$?
  else
    "$here/should-release.sh" < /dev/null || code=$?
  fi
  if [[ $code -ne $want ]]; then
    echo "FAIL $name: exit $code, want $want"
    fail=1
  else
    echo "ok   $name"
  fi
}

check "a fix" 0 "fix: plan the theme"
check "a scoped fix" 0 "fix(theme): plan the theme (#28)"
check "a feature" 0 "feat: add a location"
check "a scoped feature" 0 "feat(cluster): add a location"
check "a spec sync" 0 "chore: sync OpenAPI spec from app v3.10.0 (#27)"
check "a squash under a loose title" 0 "Make the theme plan right (#30)"
check "a fix among docs" 0 "docs: a note" "fix: a bug"
check "a breaking mark" 0 "chore!: drop an attribute"
check "a scoped breaking mark" 0 "docs(api)!: rename a field"
check "a breaking footer on a docs commit" 0 $'docs: a note\n\nBREAKING CHANGE: a field went'
check "a fix listed in a chore squash" 0 $'chore: tidy (#31)\n\n* fix: a bug\n* docs: a note'
check "a breaking mark listed in a chore squash" 0 $'chore: tidy (#31)\n\n* feat!: drop a field'
check "a merge commit of a fix" 0 $'Merge pull request #32 from a/b\n\nfix: a bug'
check "a merge commit of a loose title" 0 $'Merge pull request #32 from a/b\n\nMake it right'
check "a loose bullet in a chore squash" 0 $'chore: tidy (#35)\n\n* update the provider'
check "a breaking footer in a chore squash" 0 $'chore: tidy (#36)\n\n* chore: one\n\nBREAKING-CHANGE: a field went'
check "a breaking footer on a merge commit" 0 $'Merge pull request #37 from a/b\n\ndocs: a note\n\nBREAKING CHANGE: a field went'
check "docs alone" 1 "docs: a note"
check "CI alone" 1 "ci: bump a step" "build(deps): bump a tool"
check "tests and chores alone" 1 "test: cover it" "chore: tidy" "style: format"
check "a merge commit of docs" 1 $'Merge pull request #33 from a/b\n\ndocs: a note'
check "a chore squash listing chores" 1 $'chore: tidy (#34)\n\n* chore: one\n* docs: two'
check "nothing" 1

exit $fail
