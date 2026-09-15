#!/usr/bin/env bash
# Run: bash scripts/release/resolve-spec-ref_test.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0

# check NAME WANT_STDOUT WANT_EXIT DISPATCHED_REF LATEST_RELEASE
check() {
  local name=$1 want=$2 want_code=$3 dispatched=$4 latest=$5
  local got code
  got=$("$here/resolve-spec-ref.sh" "$dispatched" "$latest" 2>/dev/null)
  code=$?
  if [[ "$got" != "$want" || $code -ne $want_code ]]; then
    echo "FAIL $name: got '$got' (exit $code), want '$want' (exit $want_code)"
    fail=1
  else
    echo "ok   $name"
  fi
}

check "a dispatched release tag wins" v3.5.6 0 v3.5.6 v3.5.5
check "no dispatch (weekly run) syncs the latest release" v3.5.5 0 "" v3.5.5
check "a branch name is refused, so a sync can never pull main" "" 2 main v3.5.5
check "a suffixed tag is refused, since app never releases pre-releases" "" 2 v3.5.6-rc1 v3.5.5
check "a tag buried in other text is refused" "" 2 refs/tags/v3.5.6 v3.5.5

exit $fail
