#!/usr/bin/env bash
# Prints the branches a spec sync serves as a JSON array: main always, and
# release-0.x until v1.0.0 exists, since that line gets no releases after it. A
# release-0.x that is absent on origin is left out, so a missing branch does not
# fail a sync leg.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
git=${GIT_BIN:-git}
exists=$("$here/tag-exists.sh" v1.0.0)
if [[ "$exists" == yes ]]; then
  echo '["main"]'
  exit 0
fi
# Only git's exit code 2 (no matching ref) means absent; any other failure stops the plan.
code=0
"$git" ls-remote --exit-code --heads origin refs/heads/release-0.x >/dev/null || code=$?
case $code in
  0) echo '["main","release-0.x"]' ;;
  2) echo '["main"]' ;;
  *)
    echo "could not look up release-0.x on origin" >&2
    exit 1
    ;;
esac
