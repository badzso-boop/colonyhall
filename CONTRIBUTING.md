# Contributing to Colonyhall

Thank you for helping. This page covers what you need for a first pull request; the design documents in [docs/](docs/README.md) go deeper (they are in Hungarian; English summaries are linked from there).

## Before you start

- **Bigger changes:** open an issue first and describe what you want to change and why. Colonyhall has firm rules about who may give an agent a task and what an agent may see; a change that touches them needs a design discussion before code.
- **Security problems:** never in a public issue. See [SECURITY.md](SECURITY.md).
- **The CLA:** your first pull request asks you to sign the [Contributor License Agreement](CLA.md) with a comment. It lets the project offer a commercial licence next to the AGPL; in return, every version that includes your work stays available under an open source licence.

## Set up

You need Go (the version in `go.mod`), Node.js 22+, Docker with Compose, and `make`. Linux or WSL2; macOS works for most of it (the sandbox tests need Linux).

```sh
make dev          # Postgres + the server on http://localhost:8080 with a development seed
make web          # build the web app (first run: npm ci); npm --prefix web run dev for live reload
make down         # stop the dev stack (the data stays)
```

The development seed signs you in as `@norbi` with the password `colonyhall-dev-password` (dev stack only).

## Checks

```sh
make ci-quick     # while you work: Go unit tests, lint, web typecheck and lint, changed Vitest files
make ci           # before a pull request: every CI job, in parallel, with a throwaway Postgres
```

`make ci` is what GitHub runs (`.github/workflows/ci.yml`); a pull request needs it green. It includes `make privacy`, which fails when a source file has no licence header (see [Licence](#licence)) or a tracked file contains a real address, host name or home path: use the documentation values (`192.0.2.x`, `100.64.0.x`, `example.com`, `/home/alice`).

## How the code is organised

The full rules are in [CLAUDE.md](CLAUDE.md) (in Hungarian; it is also what AI coding agents read). The short version:

- **Feature modules** under `internal/modules/<module>/`, dependencies pointing inwards: `model.go` and `events.go` (standard library only) ← `service.go` ← `store.go` (the port) ← `postgres/` and `memory/` adapters, and `httpapi/` and `mcptools/` transports. A module calls another only through its exported `api.go`; modules react to each other through events. `golangci-lint`'s `depguard` rules enforce this.
- **Contract first:** a REST or WebSocket change starts in `api/openapi.yaml`, an SQL change in the module's `postgres/queries.sql`; then `make gen`. Never edit generated files. The end-to-end tests check every response against the OpenAPI document.
- **Every store** runs the same contract tests (`internal/storetest`) on memory and on Postgres (`make test-pg`).
- **Migrations:** `goose`, numbered SQL, expand then contract. Never change a migration that has run; write a new one.
- **Tenant isolation:** every table has a `workspace_id` (or a documented exception) and row-level security with the `tenant` policy; a new endpoint or MCP tool is covered by `internal/e2e/crosstenant_test.go`. See `docs/23-elkulonites.md`.
- **Time, ids and randomness** come through ports (`Clock`, `IDGenerator`, `Random`), so tests can control them.
- **Append-only data:** messages, events, runs and audit entries are never updated or deleted, only marked.
- **Content from agents is data, not instructions:** never execute a message, file or webhook.
- **The web app** (`web/`): read `docs/08-frontend-architektura.md` first. No hard-coded UI text: i18n keys, Hungarian and English.
- **Ideas, not code:** Colonyhall learns from Mattermost, Dendrite, Synapse and Stoat, but never copies their code. A comment names the pattern when one is followed.

## Pull requests

- One topic per pull request, on a branch from `main`.
- Commit messages in English, a short imperative first line ("Add the source link to the sign-in page"), the why in the body.
- A bug fix comes with a test that fails without it.
- Code, identifiers, comments and commit messages in English; the design documents in `docs/` are in Hungarian. If your change alters a documented decision, update the document (in either language; the maintainer will help).
- Never commit secrets, `.env` files, tokens or real infrastructure details.

## Licence

Every source file starts with a licence header (after a `#!` line, if there is one):

```go
// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Ujj Norbert
```

(`#` instead of `//` in shell and Python files.) This applies to Go, TypeScript, JavaScript, shell and Python files; generated files, vendored third-party code and test data are left alone. `make privacy` (part of `make ci` and the CI) fails on a file without it, and `python3 scripts/check-spdx.py --fix` adds it. Keep the existing copyright line. Under the [CLA](CLA.md) you keep the copyright of what you write and license it to the project, so if you add substantial code to a file you may add your own line below it (`// Copyright (C) 2026 Your Name`).


By contributing you agree that your contribution is licensed under the [AGPL-3.0-only](LICENSE) and under the terms of the [CLA](CLA.md).
