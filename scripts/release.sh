#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Ujj Norbert

# Build the release artifacts: colony and colonyhall for every platform,
# SHA256SUMS and install.sh, into OUT (default dist/).
#
#   scripts/release.sh v0.3.0 [OUT]
#
# The GitHub workflow (.github/workflows/release.yml) and `make release-local`
# both run this, so a local build matches a release. SKIP_WEB=1 reuses an
# existing web/dist; TARGETS overrides the platform list.
set -eu

version=${1:?usage: scripts/release.sh VERSION [OUT]}
out=${2:-dist}
targets=${TARGETS:-"linux/amd64 linux/arm64 darwin/amd64 darwin/arm64"}

case $version in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "release: version must look like v1.2.3 (got $version)" >&2; exit 2 ;;
esac

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

if [ "${SKIP_WEB:-0}" != 1 ]; then
  npm --prefix web ci --no-audit --no-fund
  npm --prefix web run build
fi
[ -f web/dist/index.html ] || { echo "release: web/dist is missing (build the PWA first)" >&2; exit 1; }

rm -rf "$out"
mkdir -p "$out"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

ldflags="-s -w -X colonyhall/internal/server.Version=$version -X colonyhall/internal/guard.Version=$version"
for t in $targets; do
  os=${t%/*}
  arch=${t#*/}
  for bin in colony colonyhall; do
    dir="$work/${bin}_${version}_${os}_${arch}"
    mkdir -p "$dir"
    CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "$ldflags" -o "$dir/$bin" "./cmd/$bin"
    tar -C "$dir" -czf "$out/${bin}_${version}_${os}_${arch}.tar.gz" "$bin"
  done
done
if [ -f scripts/install.sh ]; then
  cp scripts/install.sh "$out/install.sh"
fi

(
  cd "$out"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- * > SHA256SUMS
  else
    shasum -a 256 -- * > SHA256SUMS
  fi
)
echo "release: $version artifacts in $out"
