#!/bin/sh
# Tests of the CI's own scripts: the path filter (ci-changes.sh) and the final
# gate (ci-ok.sh). Run by `make ci-selftest` and the CI's actionlint job.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
fails=0

# expect MODE "file..." "key=value..."
expect() {
  mode=$1 files=$2 want=$3
  got=$(printf '%s' "$files" | tr ' ' '\n' | "$here/ci-changes.sh" "$mode")
  for kv in $want; do
    if ! printf '%s\n' "$got" | grep -qx "$kv"; then
      echo "FAIL ci-changes $mode [$files]: want $kv, got: $(printf '%s' "$got" | tr '\n' ' ')"
      fails=$((fails + 1))
    fi
  done
}

expect pr "docs/06-allapot-es-uzemeltetes.md README.md task.md" \
  "go=false web=false gen=false build=false docker=false race=false vuln=false shell=false actions=false"
expect pr "web/src/app/App.tsx" "web=true go=false gen=true build=true docker=true race=false vuln=false"
expect pr "web/package-lock.json" "web=true vuln=true"
expect pr "web/embed.go" "go=true web=false"
expect pr "internal/modules/conversation/service.go" "go=true web=false gen=true race=false install=false"
expect push "internal/modules/conversation/service.go" "go=true race=true"
expect push "web/src/app/App.tsx" "race=false"
expect pr "internal/httpserver/web/index.html" "devpage=true go=true"
expect pr "scripts/install.sh" "shell=true install=true go=false"
expect pr "internal/setup/cli.go" "install=true go=true"
expect pr "deploy/Dockerfile" "docker=true build=true go=false web=false"
expect pr "api/openapi.yaml" "go=true web=true gen=true"
expect pr ".github/workflows/release.yml" "actions=true go=false"
expect pr "go.sum" "go=true vuln=true"
expect pr "some/unknown/file.txt" "go=true"
expect pr "Makefile" "go=true web=true devpage=true shell=true install=true actions=true docker=true race=true vuln=true"
expect pr ".github/workflows/ci.yml" "go=true web=true actions=true"
expect all "" "go=true web=true race=true vuln=true install=true browser=false"
expect nightly "" "go=false web=false build=false race=true vuln=true"

# gate NEEDS-JSON want-exit description
gate() {
  if NEEDS=$1 "$here/ci-ok.sh" >/dev/null 2>&1; then rc=0; else rc=1; fi
  if [ "$rc" != "$2" ]; then
    echo "FAIL ci-ok: $3 (exit $rc, want $2)"
    fails=$((fails + 1))
  fi
}

# Build a needs object: every job with one result, then overrides.
needs() { # results-for-filtered-jobs outputs-value [jq-overrides]
  jq -n --arg r "$1" --arg v "$2" '
    {changes: {result: "success", outputs: ({go:$v, web:$v, devpage:$v, shell:$v, install:$v,
      actions:$v, gen:$v, build:$v, docker:$v, race:$v, vuln:$v, browser:"false"})}} +
    ([ "go-test","go-e2e","go-race","go-pg","go-lint","gen-check","web-typecheck","web-lint",
       "web-test","devpage-test","shellcheck","actionlint","vuln","build","install-test",
       "docker-image" ] | map({key: ., value: {result: $r}}) | from_entries) +
    {"e2e-browser": {result: "skipped"}}' | jq -c "${3:-.}"
}

gate "$(needs success true)" 0 "everything ran and passed"
gate "$(needs skipped false)" 0 "a docs-only change: everything skipped by the filter"
gate "$(needs success true '."go-e2e".result = "failure"')" 1 "a failed job"
gate "$(needs success true '."web-test".result = "cancelled"')" 1 "a cancelled job"
gate "$(needs success true '."go-pg".result = "skipped"')" 1 "a job skipped although its filter said run"
gate "$(needs skipped false '.changes.result = "failure"')" 1 "the changes job failed"
gate "$(needs success true 'del(."vuln")')" 1 "a job missing from needs"
gate "$(needs success true '. + {"new-job": {result: "success"}}')" 1 "a job not known to ci-ok.sh"
gate "$(needs success true '.changes.outputs.browser = "true"')" 1 "browser tests switched on but skipped"
gate "$(needs success true '.changes.outputs.browser = "true" | ."e2e-browser".result = "success"')" 0 "browser tests switched on and passed"

# Every script is executable in git: on a Windows drive (core.fileMode=false)
# chmod does not reach the index, and the workflows run them directly.
for f in $(git -C "$here/.." ls-files -s -- 'scripts/*.sh' | awk '$1 != "100755" {print $4}'); do
  echo "FAIL $f is not executable in git: git update-index --chmod=+x $f"
  fails=$((fails + 1))
done

if [ "$fails" -gt 0 ]; then
  echo "$fails failure(s)"
  exit 1
fi
echo "ci-selftest: ok"
