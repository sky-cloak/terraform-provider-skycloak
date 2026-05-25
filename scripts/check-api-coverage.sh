#!/usr/bin/env bash
# Lists generated API operations that this consumer does NOT yet use — i.e. API
# surface (often newly added) that may warrant a new resource/data source or
# facade method. Informational: always exits 0.
set -euo pipefail

GEN="internal/apiclient/apiclient.gen.go"
if [ ! -f "$GEN" ]; then
  echo "generated client not found at $GEN" >&2
  exit 0
fi

defined=$(grep -oE 'func \(c \*ClientWithResponses\) [A-Za-z0-9]+WithResponse' "$GEN" \
  | sed -E 's/.* ([A-Za-z0-9]+)WithResponse/\1/' \
  | grep -v 'WithBody$' | sort -u)

missing=()
while read -r op; do
  [ -z "$op" ] && continue
  refs=$(grep -rE "${op}WithResponse" --include='*.go' . | grep -vc 'apiclient.gen.go' || true)
  if [ "${refs:-0}" -eq 0 ]; then
    missing+=("$op")
  fi
done <<< "$defined"

total=$(echo "$defined" | grep -c . || true)
used=$(( total - ${#missing[@]} ))
echo "API operation coverage: ${used}/${total} generated operations used by this consumer."
if [ ${#missing[@]} -gt 0 ]; then
  echo ""
  echo "Not yet exposed (${#missing[@]}):"
  printf '  - %s\n' "${missing[@]}"
fi
