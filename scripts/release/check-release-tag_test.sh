#!/usr/bin/env bash
# Run: bash scripts/release/check-release-tag_test.sh
# Builds a throwaway repository in a temp folder: no network, nothing pushed.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com

cd "$work" || exit 1
git init -q -b main .
git commit -q --allow-empty -m "base"
git commit -q --allow-empty -m "on main only"
main_commit=$(git rev-parse HEAD)
git checkout -q -b line HEAD~1
git commit -q --allow-empty -m "on release-0.x only"
line_commit=$(git rev-parse HEAD)
git checkout -q -b stray HEAD~1
git commit -q --allow-empty -m "on no release branch"
stray_commit=$(git rev-parse HEAD)
# Stand in for the remote branches a full clone has.
git update-ref refs/remotes/origin/main "$main_commit"
git update-ref refs/remotes/origin/release-0.x "$line_commit"

# tag NAME COMMIT
tag() { git tag "$1" "$2"; }

# check NAME WANT_EXIT WANT_MESSAGE TAG [ACTOR]
check() {
  local name=$1 want_code=$2 want_msg=$3 t=$4 actor=${5:-}
  local err code
  err=$("$here/check-release-tag.sh" "$t" "$actor" 2>&1 >/dev/null)
  code=$?
  if [[ $code -ne $want_code || "$err" != *"$want_msg"* ]]; then
    echo "FAIL $name: exit $code, '$err', want exit $want_code with '$want_msg'"
    fail=1
  else
    echo "ok   $name"
  fi
}

tag v0.6.0 "$main_commit"
tag v0.6.1 "$line_commit"
tag v0.6.2 "$stray_commit"
check "a v0 tag on main is accepted" 0 "" v0.6.0
check "a v0 tag on release-0.x is accepted" 0 "" v0.6.1
check "a v0 tag on neither main nor release-0.x is refused" 1 "neither main nor release-0.x" v0.6.2

check "a tag without the v prefix is refused" 2 "not a release tag" 0.6.0
check "a tag with only two parts is refused" 2 "not a release tag" v0.6
check "a suffixed tag is refused" 2 "not a release tag" v0.6.0-rc1
check "a branch name is refused" 2 "not a release tag" main
check "no tag is refused" 2 "not a release tag" ""

# v1 and later never take the pre-v1 path: before v1.0.0 exists a v1.x tag is refused
tag v1.0.1 "$main_commit"
check "a v1.x tag before v1.0.0 exists is refused" 1 "v1.0.0 does not exist yet" v1.0.1
tag v2.0.0 "$main_commit"
check "a v2 tag on main is accepted" 0 "" v2.0.0
tag v2.0.1 "$line_commit"
check "a v2 tag off main is refused even on release-0.x" 1 "not on main" v2.0.1

tag v1.0.0 "$main_commit"
check "v1.0.0 from anyone but the release bot is refused" 1 "only the release-v1 workflow" v1.0.0 someone
check "v1.0.0 from the release bot on main is accepted" 0 "" v1.0.0 "skycloak-releaser-bot[bot]"
check "a v1.x tag on main is accepted once v1.0.0 exists" 0 "" v1.0.1
check "a v0 tag on main is refused once v1.0.0 exists" 1 "v1.0.0 exists" v0.6.0

exit $fail
