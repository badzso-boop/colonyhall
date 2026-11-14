#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Ujj Norbert

# Decide which CI jobs a change needs (.github/workflows/ci.yml, `changes`).
#
#   scripts/ci-changes.sh MODE < changed-files
#
# MODE is one of:
#   all       run everything (manual runs, or no usable diff)
#   nightly   only the slow and network checks: race detector, vulnerabilities
#   push      a push to main: the diff decides, plus the race detector for Go
#   pr        a pull request: the diff decides (no race detector, unless the CI changed)
#
# Prints key=true|false lines for $GITHUB_OUTPUT. A file that matches no rule
# counts as Go code, so an unknown file runs more, never less.
set -eu

mode=${1:?usage: ci-changes.sh all|nightly|push|pr}

go=false web=false devpage=false shell=false install=false actions=false
docker=false deps=false everything=false

case "$mode" in
  all) everything=true ;;
  nightly | push | pr) ;;
  *) echo "unknown mode: $mode" >&2; exit 2 ;;
esac

if [ "$mode" = push ] || [ "$mode" = pr ]; then
  while IFS= read -r f || [ -n "$f" ]; do
    [ -n "$f" ] || continue
    case "$f" in
      # The CI itself: a change here must prove every job still works.
      .github/workflows/ci.yml | Makefile | scripts/ci*.sh) everything=true ;;
      .github/*) actions=true ;;
      docs/* | *.md | LICENSE | NOTICE | .gitignore | .gitattributes) ;;
      web/package.json | web/package-lock.json) web=true deps=true ;;
      web/*.go) go=true ;;
      web/*) web=true ;;
      # The dev page is embedded in the server binary.
      internal/httpserver/web/*) devpage=true go=true ;;
      scripts/install.sh | scripts/test-install.sh) shell=true install=true ;;
      scripts/*.sh) shell=true ;;
      internal/setup/*) go=true install=true ;;
      deploy/*) docker=true ;;
      go.mod | go.sum) go=true deps=true ;;
      # The contract generates both the Go server and the PWA's types.
      api/*) go=true web=true ;;
      *) go=true ;;
    esac
  done
fi

if [ "$everything" = true ]; then
  go=true web=true devpage=true shell=true install=true actions=true docker=true deps=true
fi

gen=false build=false
if [ "$go" = true ] || [ "$web" = true ]; then gen=true build=true docker=true; fi
if [ "$docker" = true ]; then build=true; fi

race=false vuln=$deps
case "$mode" in
  all) race=true ;;
  push) race=$go ;;
  # A change to the CI itself proves every job, the slow one too.
  pr) race=$everything ;;
  nightly)
    # Nothing else: the code did not change since the last push.
    go=false web=false devpage=false shell=false install=false actions=false
    docker=false gen=false build=false race=true vuln=true ;;
esac

# Browser end-to-end tests (docs/02, "CI"): not written yet. Turning them on
# is this one line (browser=$build) plus the steps of the e2e-browser job.
browser=false

printf '%s\n' "go=$go" "web=$web" "devpage=$devpage" "shell=$shell" "install=$install" \
  "actions=$actions" "gen=$gen" "build=$build" "docker=$docker" "race=$race" "vuln=$vuln" \
  "browser=$browser"
