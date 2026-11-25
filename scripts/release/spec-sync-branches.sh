#!/usr/bin/env bash
# Prints the branches a spec sync serves as a JSON array: main always, and
# release-0.x until v1.0.0 exists, since that line gets no releases after it.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
exists=$("$here/tag-exists.sh" v1.0.0)
if [[ "$exists" == yes ]]; then
  echo '["main"]'
else
  echo '["main","release-0.x"]'
fi
