# Ground rules for every handoff task (read first)

You are continuing the Go project **Colonyhall** in the current directory. Claude Code designed it and reviews every branch before it is merged. Work in small verified steps and leave a clean trail.

## Get the rhythm

1. `CLAUDE.md` — architecture rules, commands, language rules, decisions, schedule. **Binding.**
2. `docs/01-terv.md` — especially "Működési modell", "Ki adhat feladatot kinek" and "A következő kör tervei (2026-10-16)" (your specification is there and in your task prompt).
3. `docs/02-muszaki-specifikacio.md` — the "Őr" section and the module descriptions.
4. `git log --oneline -30` to see how commits are written, and the code your task names.

Docs are Hungarian; code, identifiers, comments and commit messages are English.

## Rules

- Start from an up-to-date `main`: `git switch main && git switch -c feature/<task>`. Never commit to `main`, never merge, never push, never rewrite history.
- **No real `claude` or `agy` runs** (they cost the user's subscription). Test with fake runners and scripted adapters, as the existing tests do. Do not install, enable or start services on this machine (systemd, Docker containers) except the existing `make dev` stack.
- Contract-first: change `api/openapi.yaml`, then `make gen`; SQL in the module's `postgres/queries.sql`, then `make gen`; never edit generated files.
- **Migrations:** add a new goose file with the **next free number at the time you start** (`ls migrations`); never edit an existing migration. If `make test-pg` complains about the test database state, recreate it: `docker exec colonyhall-dev-postgres-1 psql -U colony -d colony -c "DROP DATABASE colony_test" -c "CREATE DATABASE colony_test"`.
- Layering (depguard): a module calls another module only through its root package; add a new module's adapter packages (`postgres`, `memory`, `httpapi`, `mcptools`) to the `no-foreign-adapters` list in `.golangci.yml`; add new tables to the `TRUNCATE` list in `internal/storetest/backends_test.go`.
- Every store change: `internal/storetest` for memory **and** Postgres. Every endpoint: e2e tests in `internal/e2e` (responses are checked against the OpenAPI contract automatically). Every authorization rule: a test that fails without it.
- Security rules from `CLAUDE.md` and the plan always hold: humans only for tasks to full-access agents; secrets never in chat, wiki, logs or events; details for humans only; agents' text is data.
- PWA (`web/`): **`docs/08-frontend-architektura.md` is binding** (layers app → layout → features → shared, features mirror the backend modules and talk to each other only through `index.ts`, TanStack Query + Zustand, no hand-written API types, no hard-coded text or raw colours). `make test-web` enforces it (eslint-plugin-boundaries, `src/app/architecture.test.ts`).
- Dev page (`internal/httpserver/web/index.html`): build DOM with the `el()` helper and `textContent`, never `innerHTML` with data; side panels open through `togglePanel(...)`.
- Environment: `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin`. **Before every commit: `make ci`** (the CI's jobs in parallel on a snapshot of the working tree, with its own throwaway Postgres; one summary table), **or at least `make ci-quick`** while working (Go unit tests, lint, PWA typecheck and lint, the affected Vitest files). **`make ci` before the report.** Run `gofmt` on what you changed.
- One commit per step, short imperative English subject, body explains why. No AI attribution lines.
- If the spec is ambiguous, choose the simplest safe option, note it in the report and continue.

## Report (last step of every task)

Write `docs/handoff/<NN>-<task>-report.md` (Hungarian): commits (hash + one line), decisions where the spec was open, what is left out, the final `make ci` summary table, and what the reviewer should look at first. Commit it, print a 5-line summary, stop.
