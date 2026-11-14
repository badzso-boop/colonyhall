#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Ujj Norbert

# The final CI gate (.github/workflows/ci.yml, `ci-ok`): the one required
# status check. Reads the `needs` context as JSON from $NEEDS and fails when a
# job failed or was cancelled, or was skipped although the path filter of the
# `changes` job asked for it. A job skipped by its filter counts as passed, so
# a docs-only change still gets a green check.
#
#   NEEDS='{"changes":{...},"go-test":{...}}' scripts/ci-ok.sh
set -eu

: "${NEEDS:?NEEDS must hold the needs context as JSON}"

# job:filter key. A key of "-" means the job always runs (no filter).
jobs="
changes:-
privacy:-
go-test:go
go-e2e:go
go-race:race
go-pg:go
go-lint:go
gen-check:gen
web-typecheck:web
web-lint:web
web-test:web
devpage-test:devpage
shellcheck:shell
actionlint:actions
vuln:vuln
build:build
install-test:install
docker-image:docker
e2e-browser:browser
"

bad=0
for entry in $jobs; do
  job=${entry%%:*} key=${entry#*:}
  result=$(printf '%s' "$NEEDS" | jq -r --arg j "$job" '.[$j].result // "missing"')
  case "$key" in
    -) want=true ;;
    *) want=$(printf '%s' "$NEEDS" | jq -r --arg k "$key" '.changes.outputs[$k] // "missing"') ;;
  esac
  verdict=ok
  case "$result:$want" in
    success:true | skipped:false) ;;
    # A filtered-out job that ran anyway passed; nothing wrong with that.
    success:false) ;;
    skipped:true) verdict="skipped although the change needs it" ;;
    missing:*) verdict="not in needs: add it to ci-ok's needs list" ;;
    *:missing) verdict="the changes job has no '$key' output" ;;
    *) verdict=$result ;;
  esac
  printf '%-14s %-9s %s\n' "$job" "$result" "$verdict"
  [ "$verdict" = ok ] || bad=1
done

# A job added to the workflow but not to the list above must not slip through.
extra=$(printf '%s' "$NEEDS" | jq -r 'keys[]' | while IFS= read -r j; do
  case "$jobs" in *"
$j:"*) ;; *) printf '%s ' "$j" ;; esac
done)
if [ -n "$extra" ]; then
  echo "jobs missing from scripts/ci-ok.sh: $extra"
  bad=1
fi

exit "$bad"
