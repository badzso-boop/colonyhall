#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Ujj Norbert

# Colonyhall installer: downloads the `colony` binary for this machine from a
# GitHub release, verifies it against SHA256SUMS, puts it into ~/.local/bin
# and starts `colony setup`, which does the rest (docs/15-telepites-terv.md).
#
#   sh install.sh [--version v1.2.3] [--prefix DIR] [--repo OWNER/NAME] [--no-setup] [-- SETUP ARGS]
#
# While the repository is private a read-only token is needed. It is taken
# from GITHUB_TOKEN, then `gh auth token`, then asked (hidden). It is never
# written to a file or a log and never put on a command line: curl reads it
# from stdin.
set -eu

repo=${COLONY_REPO:-badzso-boop/colonyhall}
api=${COLONY_GITHUB_API:-https://api.github.com}
web=${COLONY_GITHUB_WEB:-https://github.com}
prefix=${COLONY_PREFIX:-$HOME/.local/bin}
version=""
run_setup=1

say() { printf '%s\n' "$*" >&2; }
die() { say "colony install: $*"; exit 1; }

while [ $# -gt 0 ]; do
  case $1 in
    --version) [ $# -ge 2 ] || die "--version needs a value"; version=$2; shift 2 ;;
    --prefix) [ $# -ge 2 ] || die "--prefix needs a value"; prefix=$2; shift 2 ;;
    --repo) [ $# -ge 2 ] || die "--repo needs a value"; repo=$2; shift 2 ;;
    --no-setup) run_setup=0; shift ;;
    -h|--help)
      say "usage: sh install.sh [--version v1.2.3] [--prefix DIR] [--repo OWNER/NAME] [--no-setup] [-- SETUP ARGS]"
      exit 0 ;;
    --) shift; break ;;
    *) break ;;
  esac
done

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required; install it with your package manager and run this again"; }
need curl
need tar
need uname

os=$(uname -s)
case $os in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported system: $os (Linux, WSL2 and macOS are supported)" ;;
esac
arch=$(uname -m)
case $arch in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported processor: $arch (amd64 and arm64 are supported)" ;;
esac
wsl=""
if [ "$os" = linux ] && { [ -n "${WSL_DISTRO_NAME:-}" ] || [ -e /proc/sys/fs/binfmt_misc/WSLInterop ]; }; then
  wsl=" (WSL)"
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  die "sha256sum or shasum is required to verify the download"
fi

# The token: from the environment, from the GitHub CLI, or asked once.
token=${GITHUB_TOKEN:-}
if [ -z "$token" ] && command -v gh >/dev/null 2>&1; then
  token=$(gh auth token 2>/dev/null || true)
fi
# has_tty: a terminal can really be opened (not just a /dev/tty node).
has_tty() { (: </dev/tty) 2>/dev/null; }

asked=0
ask_token() {
  [ "$asked" = 0 ] || return 1
  asked=1
  has_tty || return 1
  say "The release could not be read without a token (the repository may be private)."
  say "Paste a read-only GitHub token (fine-grained, Contents: read); it is not shown or saved:"
  stty -echo </dev/tty 2>/dev/null || true
  IFS= read -r token </dev/tty || token=""
  stty echo </dev/tty 2>/dev/null || true
  say ""
  [ -n "$token" ]
}

# HTTPS only (and only HTTPS redirects); the tests' fake server sets
# COLONY_INSECURE_HTTP=1.
proto='=https'
[ "${COLONY_INSECURE_HTTP:-0}" = 1 ] && proto='=https,http'

# fetch URL OUT [ACCEPT]: GET with the token (if any) from stdin, never in argv.
# curl does not send the Authorization header on to a redirect's other host.
fetch() {
  if [ -n "$token" ]; then
    printf 'header = "Authorization: Bearer %s"\nheader = "Accept: %s"\n' "$token" "${3:-application/vnd.github+json}" |
      curl -fsSL --proto "$proto" --proto-redir "$proto" --config - -o "$2" "$1"
  else
    curl -fsSL --proto "$proto" --proto-redir "$proto" -H "Accept: ${3:-application/vnd.github+json}" -o "$2" "$1"
  fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

# release_json: the release's API answer (latest or the pinned tag).
release_json() {
  if [ -n "$version" ]; then url="$api/repos/$repo/releases/tags/$version"; else url="$api/repos/$repo/releases/latest"; fi
  fetch "$url" "$tmp/release.json" && return 0
  ask_token && fetch "$url" "$tmp/release.json" && return 0
  die "cannot read the release from $url (check the version, the repository name and the token)"
}
release_json
[ -n "$version" ] || version=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp/release.json" | head -n 1)
[ -n "$version" ] || die "the release has no tag name"

archive="colony_${version}_${os}_${arch}.tar.gz"

# asset_url NAME: the API URL of a release asset (works for private repos).
asset_url() {
  tr ',' '\n' <"$tmp/release.json" | awk -v want="$1" '
    /"url"[[:space:]]*:/ { u = $0; sub(/.*"url"[[:space:]]*:[[:space:]]*"/, "", u); sub(/".*/, "", u) }
    /"name"[[:space:]]*:/ { n = $0; sub(/.*"name"[[:space:]]*:[[:space:]]*"/, "", n); sub(/".*/, "", n); if (n == want && u ~ /\/assets\//) { print u; exit } }'
}

download() {
  if [ -n "$token" ]; then
    u=$(asset_url "$1")
    [ -n "$u" ] || die "the release $version has no $1"
    fetch "$u" "$tmp/$1" application/octet-stream
  else
    fetch "$web/$repo/releases/download/$version/$1" "$tmp/$1" application/octet-stream
  fi
}

say "Colonyhall $version for $os/$arch$wsl"
download SHA256SUMS
download "$archive"

want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
[ -n "$want" ] || die "SHA256SUMS has no line for $archive"
got=$(sha256 "$tmp/$archive")
[ "$want" = "$got" ] || die "checksum mismatch for $archive: expected $want, got $got (nothing was installed)"

tar -xzf "$tmp/$archive" -C "$tmp" colony
mkdir -p "$prefix"
cp "$tmp/colony" "$prefix/colony.new"
chmod 755 "$prefix/colony.new"
mv -f "$prefix/colony.new" "$prefix/colony"
say "Installed $prefix/colony ($("$prefix/colony" --version 2>/dev/null || echo "$version"))"

case ":$PATH:" in
  *":$prefix:"*) ;;
  *) say "Note: $prefix is not on your PATH; add it in your shell profile (for example: export PATH=\"$prefix:\$PATH\")." ;;
esac

[ "$run_setup" = 1 ] || exit 0
token=""
# Run the setup on the terminal even when this script came through a pipe.
if [ ! -t 0 ] && has_tty; then
  exec "$prefix/colony" setup "$@" </dev/tty
fi
exec "$prefix/colony" setup "$@"
