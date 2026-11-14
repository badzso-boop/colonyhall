#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Ujj Norbert

# Run the installer tests (internal/setup) inside throwaway containers of the
# supported Linux distributions: install.sh with the system's own sh, curl
# and tar against the tests' fake GitHub. Needs Docker; not part of `make test`.
#
#   scripts/test-install.sh [IMAGE...]
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
images=${*:-"debian:12 ubuntu:24.04 fedora:41"}
bin="$root/dist/test/setup.test"
mkdir -p "$root/dist/test"
CGO_ENABLED=0 go test -c -o "$bin" ./internal/setup

for image in $images; do
  echo "== $image"
  docker run --rm -v "$root":/src:ro -w /src/internal/setup "$image" sh -c '
    set -e
    if ! command -v curl >/dev/null 2>&1; then
      if command -v apt-get >/dev/null 2>&1; then apt-get update -qq >/dev/null && apt-get install -y -qq curl ca-certificates >/dev/null
      elif command -v dnf >/dev/null 2>&1; then dnf install -y -q curl >/dev/null
      fi
    fi
    command -v tar >/dev/null 2>&1 || { command -v dnf >/dev/null 2>&1 && dnf install -y -q tar >/dev/null; }
    /src/dist/test/setup.test -test.run "TestInstallSh" -test.v 2>&1 | grep -E "^(=== RUN|--- |PASS|FAIL|ok)" || exit 1
  '
done
