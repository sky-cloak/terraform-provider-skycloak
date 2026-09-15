#!/usr/bin/env bash
# Run: bash scripts/release/next-version_test.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0

# expect NAME WANT_STDOUT WANT_EXIT GOT_STDOUT GOT_EXIT
expect() {
  if [[ "$4" != "$2" || $5 -ne $3 ]]; then
    echo "FAIL $1: got '$4' (exit $5), want '$2' (exit $3)"
    fail=1
  else
    echo "ok   $1"
  fi
}

# check NAME WANT_STDOUT WANT_EXIT LAST_TAG [COMMIT_MESSAGE...]
check() {
  local name=$1 want=$2 want_code=$3 tag=$4
  shift 4
  local got code
  got=$(printf '%s\0' "$@" | "$here/next-version.sh" "$tag" 2>/dev/null)
  code=$?
  expect "$name" "$want" "$want_code" "$got" "$code"
}

check "a fix bumps the patch" v0.12.2 0 v0.12.1 "fix(realms): keep the cluster id"
check "a feat bumps the minor and resets the patch" v0.13.0 0 v0.12.1 "fix: a" "feat(roles): manage application roles"
check "a breaking change marked with ! refuses to release" "" 3 v0.12.1 "fix: a" "feat(api)!: drop the old field"
check "a BREAKING CHANGE footer refuses to release" "" 3 v0.12.1 \
  $'fix(api): rename a field\n\nBREAKING CHANGE: clients must send name, not display_name'

check "a last tag that is not vX.Y.Z is refused" "" 2 "v1.2" "fix: a"
check "no last tag at all is refused" "" 2 "" "fix: a"

# No commits since the tag: tagging again would republish identical code.
got=$(: | "$here/next-version.sh" v0.12.1 2>/dev/null); code=$?
expect "nothing since the last tag refuses to release" "" 4 "$got" "$code"

# Real `git log --format=%B%x00` output puts a newline between records, so every
# commit after the newest starts with one. Build a throwaway repo to feed it.
repo=$(mktemp -d)
trap 'rm -rf "$repo"' EXIT
gitc() { git -C "$repo" -c user.name=t -c user.email=t@t "$@"; }
gitc init -q
gitc commit -q --allow-empty -m "chore: start"
gitc tag v0.12.1
gitc commit -q --allow-empty -m "feat: older"
gitc commit -q --allow-empty -m "fix: newest"
got=$(gitc log --format='%B%x00' v0.12.1..HEAD | "$here/next-version.sh" v0.12.1 2>/dev/null); code=$?
expect "a feat that is not the newest commit still counts in real git log output" v0.13.0 0 "$got" "$code"

exit $fail
