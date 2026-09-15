#!/usr/bin/env bash
# Run: bash scripts/release/resolve-spec-ref_test.sh
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0

# check NAME WANT_STDOUT WANT_EXIT REF
check() {
  local name=$1 want=$2 want_code=$3 ref=$4
  local got code
  got=$("$here/resolve-spec-ref.sh" "$ref" 2>/dev/null)
  code=$?
  if [[ "$got" != "$want" || $code -ne $want_code ]]; then
    echo "FAIL $name: got '$got' (exit $code), want '$want' (exit $want_code)"
    fail=1
  else
    echo "ok   $name"
  fi
}

check "a release tag is accepted" v3.5.6 0 v3.5.6
check "a multi-digit release tag is accepted" v10.17.123 0 v10.17.123
check "no ref is refused, so a run can never guess a release" "" 2 ""
check "a branch name is refused, so a sync can never pull main" "" 2 main
check "a suffixed tag is refused, since app never releases pre-releases" "" 2 v3.5.6-rc1
check "a tag buried in other text is refused" "" 2 refs/tags/v3.5.6

exit $fail
