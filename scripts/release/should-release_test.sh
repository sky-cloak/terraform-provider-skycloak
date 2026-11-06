#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fails=0
check() { # want(0 release, 1 none) label subjects
  local got=0
  printf '%s' "$3" | "$here/should-release.sh" || got=$?
  if [[ "$got" != "$1" ]]; then
    echo "FAIL $2: want $1, got $got"
    fails=$((fails + 1))
  fi
}
check 0 "a fix" $'fix: plan the theme\n'
check 0 "a scoped fix" $'fix(theme): plan the theme (#28)\n'
check 0 "a feature" $'feat: add a location\n'
check 0 "a scoped feature" $'feat(cluster): add a location\n'
check 0 "a breaking change goes on to stop" $'chore!: drop an attribute\n'
check 0 "a scoped breaking change" $'docs(api)!: rename a field\n'
check 0 "a spec sync" $'chore: sync OpenAPI spec from app v3.10.0 (#27)\n'
check 0 "a squash under a loose title" $'Make the theme plan right (#30)\n'
check 0 "a fix among docs" $'docs: a note\nfix: a bug\n'
check 0 "no final line break" 'fix: a bug'
check 1 "docs alone" $'docs: a note\n'
check 1 "CI alone" $'ci: bump a step\nbuild(deps): bump a tool\n'
check 1 "tests and chores alone" $'test: cover it\nchore: tidy\nstyle: format\n'
check 1 "nothing" ''
[[ $fails -eq 0 ]] && echo "ok"
exit "$fails"
