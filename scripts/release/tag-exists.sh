#!/usr/bin/env bash
# Usage: tag-exists.sh <tag>
# Prints "yes" or "no" for whether the tag is on origin. Only git's exit code 2
# (no matching ref) means absent; any other failure, such as a network or auth
# error, is an error and never reads as "no tag".
set -uo pipefail
tag=${1:-}
if [[ -z "$tag" ]]; then
  echo "usage: tag-exists.sh <tag>" >&2
  exit 64
fi
"${GIT_BIN:-git}" ls-remote --exit-code --tags origin "refs/tags/$tag" >/dev/null
case $? in
  0) echo yes ;;
  2) echo no ;;
  *)
    echo "could not look up tag $tag on origin" >&2
    exit 1
    ;;
esac
