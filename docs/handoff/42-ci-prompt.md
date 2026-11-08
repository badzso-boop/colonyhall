# Handoff prompt 42: continuous integration, as parallel as possible

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/ci ../new-chat-worktrees/ci main
cd ../new-chat-worktrees/ci
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/42-ci-prompt.md)"
```

---

Branch: `feature/ci`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/ci`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push. You cannot see a workflow run on GitHub (you do not push): validate everything locally (part 5) and leave the first real run to Norbi.

After each part: `make test`, `make lint`, `actionlint` on every workflow (via its Docker image, `rhysd/actionlint`, pinned), `shellcheck` on new scripts. **Commit only when they pass.**

## Why (Norbi, 2026-11-07)

Today every check runs by hand on Norbi's PC, before a commit. The only workflow on GitHub builds releases (round 41, `.github/workflows/release.yml`, on the `feature/installer` branch until it is merged). With a colleague and later other contributors (open source or a subscription product), a broken change must not reach `main` or production unnoticed.

Norbi's wishes:

- **Run as many checks in parallel as possible.** Only what really builds on something waits for it.
- **Browser end-to-end tests come later.** They matter, but the UI still changes a lot. Design their place in the pipeline now; do not write them in this round.

## The checks today (measure them, these are Claude Code's rough numbers)

| Check | Command | Needs | About |
| --- | --- | --- | --- |
| Go unit and contract tests (memory stores) | `make test` minus `internal/e2e` | Go | 1 min |
| Go end-to-end tests | `go test ./internal/e2e/` | Go | 1–2 min |
| Race detector | `go test -race ./internal/guard/ ./internal/e2e/ ./internal/gateway/ ...` | Go | 3 min |
| Postgres store contract | `make test-pg` | a Postgres (pgvector image from round 36 on) | 1 min |
| Lint (depguard layer rules) | `make lint` | golangci-lint | 1 min |
| Generated code up to date | `make gen-check` | oapi-codegen, openapi-typescript (npm), sqlc (Docker today) | 1 min |
| Dev page tests | `node --test internal/httpserver/web/*.test.mjs` | Node | seconds |
| PWA typecheck | `npm --prefix web run typecheck` | npm ci | 1 min |
| PWA lint | `npm --prefix web run lint` | npm ci | 1 min |
| PWA Vitest | `npm --prefix web test` (80+ files, jsdom) | npm ci | **5–6 min**, the longest |
| Shell scripts | `make shellcheck` (installer branch) | shellcheck | seconds |
| Installer in distro containers | `make test-install` (installer branch) | Docker, a built `colony` | 3 min |
| Known vulnerabilities | `govulncheck ./...`, `npm audit --omit=dev` | network | 1 min |
| PWA build and server binary | `npm run build`, `go build` (the binary embeds `web/dist`) | npm, Go | 1–2 min |

## 1. The pipeline (`.github/workflows/ci.yml`)

- **Triggers:** push to `main`, pull requests to `main`, manual run. Concurrency: one run per branch, a newer push cancels the older run (`cancel-in-progress`).
- **Parallel from the start** (no `needs`), each its own job:
  - `go-test` (everything except `internal/e2e`);
  - `go-e2e`;
  - `go-race` (see the cost rules below);
  - `go-pg` (a Postgres **service container**, the same image as the dev stack; the pgvector image once round 36 needs it);
  - `go-lint`; `gen-check`;
  - `web-typecheck`; `web-lint`;
  - **`web-test` sharded** with Vitest's `--shard=i/n` in a matrix (find the `n` where adding a shard stops paying off; measure);
  - `devpage-test`; `shellcheck`; `actionlint`;
  - `vuln` (`govulncheck` pinned, `npm audit --omit=dev`; a finding fails it).
- **What builds on something** (with `needs`):
  - `build`: the PWA build, then the server and `colony` binaries with the real embedded `web/dist`; uploads them as artifacts for the jobs after it. It can start at once; it does not wait for the tests;
  - `install-test` (`make test-install`) uses the `build` artifact;
  - a **Docker image build** (no push) uses the repository, after `build`, to catch a broken `deploy/Dockerfile`;
  - **`ci-ok`**: a final job that `needs` every job above and fails if any failed or was skipped wrongly. **This is the one required status check** for branch protection, so adding a job later does not mean changing the protection rules.
- **Caches:** Go modules and build cache (`actions/setup-go` cache), the golangci-lint cache, npm (`actions/setup-node` cache with `web/package-lock.json`), the Docker layers where it helps. Reproducible: `npm ci`, pinned tool versions (golangci-lint, sqlc, govulncheck, actionlint, shellcheck).
- **The same rules as the release workflow (round 41):** third-party actions pinned to a commit (tag in a comment), `permissions: {}` at the top and only `contents: read` per job (nothing here needs to write), no `${{ }}` expressions inside `run:` scripts that come from user-controlled input (branch names, PR titles): pass them through `env:`.
- **Pull requests from forks** (for when the repository is public): `pull_request` only, never `pull_request_target` with a checkout of the PR's code, no secrets in this workflow at all.

## 2. Cost and speed (the repository is private: GitHub-hosted minutes are limited)

- Report the measured minutes per run (the sum over jobs, which is what is billed) and a monthly estimate for, say, 10 pushes a day.
- **Path filters** decide what runs: a docs-only change runs nothing but `ci-ok`; a web-only change skips the Go jobs, and so on. Implement it so `ci-ok` still passes (e.g. a small `changes` job with `dorny/paths-filter` pinned, or plain `git diff` in a script), and the skipped jobs count as passed.
- **The race detector** runs on `main` and on a nightly schedule, not on every pull request push, unless measuring shows it is cheap enough.
- **No self-hosted runner** (Norbi's PC, the homelab) in this round: a public repository would run strangers' code on his machines. Write in the report what it would take and when it would make sense.

## 3. `make ci`: the same graph locally

Agents in this project never push, so CI on GitHub runs only after Norbi pushes. Give everyone the same checks locally:

- `make ci` runs the jobs of part 1 that can run locally, **in parallel** (`make -j`, or a small script that starts them together and collects the results), each with its own log under a temp folder, and prints one summary table (job, result, seconds) at the end. It needs a Postgres: start one in a throwaway container on a free port (not the dev stack's `colony_test`, so `make ci` never wipes anything Norbi uses), and remove it afterwards.
- `make ci-quick`: the fast subset for while working (go-test, lint, web-typecheck, web-lint, the changed Vitest files).
- Update `docs/handoff/00-ground-rules.md`: "before every commit: `make ci` (or at least `make ci-quick`, and `make ci` before the report)".

## 4. Room for browser end-to-end tests later

Do **not** write browser tests now. Prepare their place:

- A design section in the docs: Playwright (the project already used `playwright-core` with a local Chromium for screenshots in rounds 33–35; the scripts lived outside the repo), against the built server with memory stores and the dev seed, as a job that `needs: build` and runs in parallel with the rest; screenshots and traces as artifacts on failure; how to keep them stable while the UI changes (role and label selectors, no CSS selectors, few smoke paths first: login, send a message, open a thread, the admin pages load).
- An empty, skipped `e2e-browser` job slot (or a commented block) with a clear TODO, wired so turning it on later does not change `ci-ok`.

## 5. Validate without pushing

- `actionlint` and `shellcheck` clean.
- Run the workflow locally with `act` (pinned, in Docker) as far as it works, or at least run every job's commands locally in the same order; say exactly what you could and could not run.
- Time each job locally and put the numbers in the report.

## 6. Branch protection and badges

- Write the exact branch protection settings for `main` that Norbi sets on GitHub (require `ci-ok`, require the branch to be up to date or not, who may bypass: Norbi pushes to `main` himself today, so say what changes for him and recommend whether to keep direct pushes for now).
- A CI badge in `README.md` (if the installer round's README is merged; otherwise note it in the report).

## Out of scope

Writing browser end-to-end tests; a self-hosted runner; deploying from CI (deployment stays manual, Norbi's decision); code coverage gates (report the current coverage once, do not enforce a number).

## Docs

Hungarian: `docs/02-muszaki-specifikacio.md` (a short CI section: the jobs and their order), `docs/06-allapot-es-uzemeltetes.md` (what a red CI means and how to read it), `CLAUDE.md` (the `make ci` and `make ci-quick` commands), `task.md`.

## Report

`docs/handoff/42-ci-report.md` (Hungarian): the commits; the job graph (a small diagram); the measured time per job and the wall-clock time of a full run; the monthly minute estimate; what `act` could and could not run; the branch protection steps for Norbi; what the reviewer should look at first (permissions, the path filters with `ci-ok`, the throwaway Postgres in `make ci`).
