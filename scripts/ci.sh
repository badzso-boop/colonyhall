#!/usr/bin/env bash
# The CI job graph on this machine (`make ci`, `make ci-quick`): the jobs of
# .github/workflows/ci.yml that can run locally, started together, each with
# its own log, and one summary table at the end.
#
#   scripts/ci.sh [--quick] [--only "job ..."] [--skip "job ..."] [--keep]
#
# The full run checks a snapshot of the working tree (HEAD plus uncommitted
# and untracked files) in a temp folder, so you can keep editing meanwhile and
# jobs that write files (gen-check, the PWA build) cannot disturb the others.
# Its Postgres is a throwaway container on a free port, never the dev stack.
# --quick runs a fast subset in place: Go unit tests, lint, PWA typecheck and
# lint, and the Vitest files affected by uncommitted changes.
set -euo pipefail

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

quick=0 only="" skip="" keep=0
while [ $# -gt 0 ]; do
  case "$1" in
    --quick) quick=1 ;;
    --only) only=$2; shift ;;
    --skip) skip=$2; shift ;;
    --keep) keep=1 ;;
    *) echo "usage: $0 [--quick] [--only \"job ...\"] [--skip \"job ...\"] [--keep]" >&2; exit 2 ;;
  esac
  shift
done

repo=$(git rev-parse --show-toplevel)
work=$(mktemp -d "${TMPDIR:-/tmp}/colonyhall-ci.XXXXXX")
logs="$work/logs"
mkdir -p "$logs"
pg=""
snapshots=()

cleanup() {
  [ -n "$pg" ] && docker rm -f "$pg" >/dev/null 2>&1 || true
  for s in "${snapshots[@]}"; do
    git -C "$repo" worktree remove --force "$s" >/dev/null 2>&1 || true
  done
  if [ "$keep" = 1 ]; then
    echo "logs kept in $logs"
  else
    rm -rf "$work"
  fi
}
trap cleanup EXIT
trap 'exit 130' INT TERM

# snapshot DIR: a detached worktree of the working tree's state. `git stash
# create` records the tracked changes (with the index's file modes, which a
# Windows drive with core.fileMode=false would lose) without touching the
# tree; the untracked files are copied after it.
snapshot() {
  local dir=$1 rev
  rev=$(git -C "$repo" stash create)
  git -C "$repo" worktree add --quiet --detach "$dir" "${rev:-HEAD}"
  snapshots+=("$dir")
  git -C "$repo" ls-files -z --others --exclude-standard |
    (cd "$repo" && tar --null -T - -cf -) | tar -C "$dir" -xf -
  # The PWA's packages come from the cache below (npm ci takes a while).
  ln -s "$modules" "$dir/web/node_modules"
  # Stage everything, so gen-check sees only what generation changes.
  git -C "$dir" add -A
}

mk() { make --no-print-directory "$@"; }

# job definitions: name -> command (run with bash -c in the job's folder).
declare -A cmd
cmd[go-test]="mk test-unit"
cmd[go-e2e]="mk test-e2e"
cmd[go-race]="mk test-race"
cmd[go-pg]="mk test-pg"
cmd[go-lint]="mk lint"
cmd[web-typecheck]="mk web-typecheck"
cmd[web-lint]="mk web-lint"
cmd[web-test]="mk web-vitest"
cmd[devpage-test]="mk test-devpage"
cmd[shellcheck]="mk shellcheck"
cmd[actionlint]="mk actionlint ci-selftest"
cmd[vuln]="mk vuln"
cmd[install-test]="mk test-install"
# These write into the tree (generated code, web/dist): their own snapshot,
# one after the other, as in CI where the image build needs the build job.
cmd[gen-check]="mk gen-check"
cmd[build]="mk web build"
cmd[docker-image]="mk docker-image"

if [ "$quick" = 1 ]; then
  jobs=(go-test go-lint web-typecheck web-lint web-test)
  cmd[web-test]="mk web-vitest VITEST_ARGS=--changed"
else
  jobs=(go-test go-e2e go-race go-pg go-lint web-typecheck web-lint web-test devpage-test
    shellcheck actionlint vuln install-test gen-check build docker-image)
fi

selected=()
for j in "${jobs[@]}"; do
  if [ -n "$only" ] && [[ " $only " != *" $j "* ]]; then continue; fi
  if [[ " $skip " == *" $j "* ]]; then continue; fi
  selected+=("$j")
done
[ ${#selected[@]} -gt 0 ] || { echo "no job selected" >&2; exit 2; }
has() { [[ " ${selected[*]} " == *" $1 "* ]]; }

# The PWA's packages for the snapshots: one install per package-lock.json,
# kept under ~/.cache (a Linux filesystem). On a Windows drive under WSL
# (/mnt/g) jsdom loads so slowly that Vitest takes 20 minutes instead of 40 s.
modules=""
if [ "$quick" = 0 ]; then
  lockhash=$(git -C "$repo" hash-object web/package-lock.json)
  cache="${XDG_CACHE_HOME:-$HOME/.cache}/colonyhall-ci/web-$lockhash"
  if [ ! -f "$cache/node_modules/.package-lock.json" ]; then
    echo "npm ci into $cache (once per package-lock.json)"
    rm -rf "$cache" && mkdir -p "$cache"
    cp "$repo/web/package.json" "$repo/web/package-lock.json" "$cache/"
    mkdir -p "$cache/scripts" && cp "$repo"/web/scripts/*.mjs "$cache/scripts/"
    npm --prefix "$cache" ci --no-audit --no-fund >"$logs/npm-ci.log" 2>&1
  fi
  modules="$cache/node_modules"
elif [ ! -d "$repo/web/node_modules" ] || [ "$repo/web/package-lock.json" -nt "$repo/web/node_modules/.package-lock.json" ]; then
  echo "npm ci (web/package-lock.json changed)"
  npm --prefix "$repo/web" ci --no-audit --no-fund >"$logs/npm-ci.log" 2>&1
fi

if [ "$quick" = 1 ]; then
  main_dir=$repo chain_dir=$repo
else
  echo "snapshot of the working tree in $work"
  main_dir="$work/src"
  snapshot "$main_dir"
  chain_dir="$work/src-build"
  if has gen-check || has build || has docker-image; then snapshot "$chain_dir"; fi
  # The snapshots' package.json is newer than the cached install now, and
  # make would run npm ci through the link (it empties the shared cache; a
  # recursive make does not inherit -o). Mark the install as fresh instead.
  touch "$modules/.package-lock.json"
fi
export -f mk

pg_env=""
if has go-pg; then
  pg="colonyhall-ci-pg-$$"
  docker run -d --rm --name "$pg" -e POSTGRES_USER=colony -e POSTGRES_PASSWORD=colony \
    -e POSTGRES_DB=colony_test --tmpfs /var/lib/postgresql/data \
    -p 127.0.0.1::5432 postgres:17-alpine >/dev/null
  port=$(docker port "$pg" 5432/tcp | head -1 | sed 's/.*://')
  for _ in $(seq 1 60); do
    docker exec "$pg" pg_isready -U colony -d colony_test >/dev/null 2>&1 && break
    sleep 0.5
  done
  pg_env="TEST_DB=postgres://colony:colony@127.0.0.1:$port/colony_test?sslmode=disable"
  cmd[go-pg]="mk test-pg '$pg_env'"
fi

declare -A pid result secs
now() { date +%s; }

run_job() { # name dir: the job's log, and its seconds in $logs/<job>.secs
  local j=$1 dir=$2 s rc=0
  s=$(now)
  (cd "$dir" && bash -c "${cmd[$j]}") >"$logs/$j.log" 2>&1 || rc=$?
  echo $(($(now) - s)) >"$logs/$j.secs"
  return "$rc"
}

started=$(now)
chain=()
for j in "${selected[@]}"; do
  case "$j" in
    gen-check | build | docker-image) chain+=("$j"); continue ;;
  esac
  run_job "$j" "$main_dir" &
  pid[$j]=$!
done

# The chain: gen-check, then build, then the image (each only when selected).
if [ ${#chain[@]} -gt 0 ]; then
  (
    for j in "${chain[@]}"; do
      if run_job "$j" "$chain_dir"; then r=ok; else r=FAIL; fi
      echo "$j $r" >>"$logs/chain.results"
      [ "$r" = ok ] || [ "$j" = gen-check ] || break
    done
  ) &
  chain_pid=$!
fi

fail=0
for j in "${!pid[@]}"; do
  if wait "${pid[$j]}"; then result[$j]=ok; else result[$j]=FAIL; fi
done
if [ ${#chain[@]} -gt 0 ]; then
  wait "$chain_pid" || true
  while read -r j r; do result[$j]=$r; done <"$logs/chain.results"
fi
for j in "${selected[@]}"; do
  [ -f "$logs/$j.secs" ] && secs[$j]=$(cat "$logs/$j.secs")
done

printf '\n%-14s %-7s %6s\n' JOB RESULT SECONDS
for j in "${selected[@]}"; do
  r=${result[$j]:-skipped}
  printf '%-14s %-7s %6s\n' "$j" "$r" "${secs[$j]:--}"
  if [ "$r" != ok ]; then
    fail=1
    [ "$r" = FAIL ] && { echo "  --- last lines of $j:"; tail -n 15 "$logs/$j.log" | sed 's/^/  /'; }
  fi
done
echo "wall clock: $(($(now) - started)) s"
if [ "$fail" = 1 ]; then
  keep=1
  exit 1
fi
