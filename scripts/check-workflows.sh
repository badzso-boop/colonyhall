#!/bin/sh
# Supply-chain rules for the GitHub workflows (round 40), checked by
# ci-selftest.sh (the CI's actionlint job). Strangers open pull requests
# against a public repository; these keep their code away from tokens.
#
#   - no pull_request_target trigger (it runs with a write token);
#   - the workflow-level permissions are {} or read-only;
#   - every action is pinned to a full commit SHA;
#   - every checkout sets persist-credentials: false;
#   - a job that can sign (id-token: write) or write contents/packages runs
#     no dependency code (npm, release.sh) itself.
#
#   scripts/check-workflows.sh [workflow.yml ...]   (default: .github/workflows/*.yml)
set -eu

here=$(cd "$(dirname "$0")" && pwd)
if [ $# -eq 0 ]; then
  set -- "$here"/../.github/workflows/*.yml
fi

fails=0
for f in "$@"; do
  out=$(awk -v F="$(basename "$f")" '
    function fail(msg) { print "FAIL " F ":" NR ": " msg; bad = 1 }
    /^[[:space:]]*#/ { next }
    /^[[:space:]]*pull_request_target[[:space:]]*:?/ { fail("pull_request_target trigger") }
    # Workflow-level permissions: {} or read-only.
    /^permissions:/ {
      if ($0 ~ /\{\}/) { toplevel = 0 } else { toplevel = 1 }
      next
    }
    toplevel && /^[^[:space:]]/ { toplevel = 0 }
    toplevel && /:[[:space:]]*write/ { fail("write permission at the workflow level: " $0) }
    # Pinned actions.
    /^[[:space:]]*-?[[:space:]]*uses:/ {
      u = $0; sub(/.*uses:[[:space:]]*/, "", u); sub(/[[:space:]].*/, "", u)
      if (u !~ /^\.\// && u !~ /@[0-9a-f]{40}$/) fail("action not pinned to a commit SHA: " u)
      if (u ~ /^actions\/checkout@/) { checkout = NR; persisted = 0 }
    }
    /persist-credentials:[[:space:]]*false/ { persisted = 1 }
    checkout && NR > checkout + 4 { if (!persisted) fail("checkout at line " checkout " without persist-credentials: false"); checkout = 0 }
    # Jobs: two-space keys under jobs:.
    /^jobs:/ { injobs = 1; next }
    injobs && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { endjob(); job = $1; sub(/:$/, "", job); privileged = 0; runsdeps = 0; next }
    injobs && /^    permissions:/ { inperm = 1; next }
    inperm && /^    [^[:space:]]/ { inperm = 0 }
    inperm && /(id-token|contents|packages|pages|attestations):[[:space:]]*write/ { privileged = 1 }
    injobs && /(npm (ci|install|run|exec)|npx |release\.sh)/ { runsdeps = 1; depline = NR }
    function endjob() {
      if (job != "" && privileged && runsdeps) { print "FAIL " F ":" depline ": job " job " can write or sign and also runs dependency code"; bad = 1 }
      job = ""
    }
    END {
      if (checkout && !persisted) fail("checkout without persist-credentials: false")
      endjob()
      exit bad
    }
  ' "$f") || true
  if [ -n "$out" ]; then
    printf '%s\n' "$out"
    n=$(printf '%s\n' "$out" | grep -c '^FAIL')
    fails=$((fails + n))
  fi
done

if [ "$fails" -gt 0 ]; then
  echo "check-workflows: $fails failure(s)"
  exit 1
fi
echo "check-workflows: ok"
