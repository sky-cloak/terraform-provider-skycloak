#!/usr/bin/env bash
# Run: bash scripts/release/release-lines_test.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# expect NAME WANT_EXIT WANT_STDOUT_OR_EMPTY -- COMMAND...
expect() {
  local name=$1 want_code=$2 want=$3
  shift 4
  local got code
  got=$("$@" 2>/dev/null)
  code=$?
  if [[ $code -ne $want_code || ( -n "$want" && "$got" != "$want" ) ]]; then
    echo "FAIL $name: got '$got' (exit $code), want exit $want_code '$want'"
    fail=1
  else
    echo "ok   $name"
  fi
}

# Stub git whose ls-remote exits with LS_CODE.
cat > "$tmp/git-stub" <<'STUB'
#!/usr/bin/env bash
exit "${LS_CODE:-0}"
STUB
# Stub gh printing RUNS as the count of successful runs.
cat > "$tmp/gh-stub" <<'STUB'
#!/usr/bin/env bash
echo "${RUNS:-0}"
STUB
chmod +x "$tmp/git-stub" "$tmp/gh-stub"
export GIT_BIN="$tmp/git-stub" GH_BIN="$tmp/gh-stub"

printf '# Changelog\n\n## [1.0.0] - 2026-12-01\n' > "$tmp/with.md"
printf '# Changelog\n\n## [Unreleased]\n' > "$tmp/without.md"

# tag-exists
expect "an existing tag reads as yes" 0 yes -- env LS_CODE=0 "$here/tag-exists.sh" v1.0.0
expect "exit 2 reads as no tag" 0 no -- env LS_CODE=2 "$here/tag-exists.sh" v1.0.0
expect "a lookup failure is an error, not no tag" 1 "" -- env LS_CODE=128 "$here/tag-exists.sh" v1.0.0
expect "no tag argument is refused" 64 "" -- "$here/tag-exists.sh"

# check-v1-release
sha=abc123
expect "a new tag with passing tests and a changelog entry is accepted" 0 "" -- env LS_CODE=2 RUNS=1 CHANGELOG="$tmp/with.md" "$here/check-v1-release.sh" v1.0.0 $sha
expect "an existing tag is refused" 1 "" -- env LS_CODE=0 RUNS=1 CHANGELOG="$tmp/with.md" "$here/check-v1-release.sh" v1.0.0 $sha
expect "a failed tag lookup is refused" 1 "" -- env LS_CODE=128 RUNS=1 CHANGELOG="$tmp/with.md" "$here/check-v1-release.sh" v1.0.0 $sha
expect "no passing test run is refused" 1 "" -- env LS_CODE=2 RUNS=0 CHANGELOG="$tmp/with.md" "$here/check-v1-release.sh" v1.0.0 $sha
expect "no changelog entry is refused" 1 "" -- env LS_CODE=2 RUNS=1 CHANGELOG="$tmp/without.md" "$here/check-v1-release.sh" v1.0.0 $sha
expect "a malformed version is refused" 64 "" -- env LS_CODE=2 RUNS=1 CHANGELOG="$tmp/with.md" "$here/check-v1-release.sh" 1.0.0 $sha

# spec-sync-branches
expect "before v1.0.0 the sync serves both lines" 0 '["main","release-0.x"]' -- env LS_CODE=2 "$here/spec-sync-branches.sh"
expect "after v1.0.0 the sync serves main only" 0 '["main"]' -- env LS_CODE=0 "$here/spec-sync-branches.sh"
expect "a failed lookup stops the sync plan" 1 "" -- env LS_CODE=128 "$here/spec-sync-branches.sh"

# check-release-tag, on a real clone with both lines
unset GIT_BIN
origin="$tmp/origin"
git init -q -b main "$origin"
g() { git -C "$origin" -c user.name=t -c user.email=t@example.com "$@"; }
g commit -q --allow-empty -m base
g branch release-0.x
g commit -q --allow-empty -m on-main
g tag v1.0.0
g tag v1.1.0
g checkout -q release-0.x
g commit -q --allow-empty -m on-0x
g tag v0.9.0
g tag -f v0.9.1 main >/dev/null
g checkout -q main
git clone -q "$origin" "$tmp/clone"
git -C "$tmp/clone" fetch -q origin release-0.x:refs/remotes/origin/release-0.x
# shellcheck disable=SC2329 # called through expect
run() { (cd "$tmp/clone" && "$here/check-release-tag.sh" "$1"); }
expect "a v0 tag on release-0.x is accepted" 0 "" -- run v0.9.0
expect "a v0 tag on main is refused" 1 "" -- run v0.9.1
expect "v1.0.0 on main is accepted" 0 "" -- run v1.0.0
expect "v1.1.0 after v1.0.0 on main is accepted" 0 "" -- run v1.1.0
git -C "$tmp/clone" tag -d v1.0.0 >/dev/null
expect "a v1 tag before v1.0.0 exists is refused" 1 "" -- run v1.1.0
expect "a non release tag is refused" 2 "" -- run latest

exit $fail
