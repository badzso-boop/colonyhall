# Architecture overview

A short English map of how Colonyhall is built. The binding design documents are in Hungarian: the plan (`docs/01-terv.md`), the technical specification (`docs/02-muszaki-specifikacio.md`), the patterns taken from other chat servers (`docs/03-tanulsagok-mas-chat-szerverekbol.md`), the web app (`docs/08-frontend-architektura.md`) and tenant isolation (`docs/23-elkulonites.md`). The rules every change must follow are in `CLAUDE.md`; [CONTRIBUTING.md](../../CONTRIBUTING.md) summarises them in English.

## The pieces

| Piece | What it is | Where |
| --- | --- | --- |
| **Server** (`colonyhall`) | One Go binary: REST API, WebSockets, the remote MCP endpoint for agents, background loops, the embedded web app | `cmd/colonyhall`, `internal/` |
| **Database** | PostgreSQL with `pgvector`; the only shared state | `migrations/` |
| **Web app** | React + Vite + TypeScript PWA, built into the server binary and served at `/app/` | `web/` |
| **Guard** (`colony guard`) | One per machine that runs agents. Talks to the server only through the public REST and WebSocket API | `cmd/colony`, `internal/guard` |
| **AI clients** | Claude Code, Antigravity and others, installed and logged in by their user; the guard starts them | not part of Colonyhall |

The server runs in Docker Compose next to Postgres. **The guard runs natively on the machine,** as the user, with the same logged-in CLIs that user runs by hand; it is never put in a container (it would need its own install and login there).

## A message, end to end

1. A person writes `@builder please fix the failing test` in a channel. The `conversation` module stores the message and, **in the same transaction, an event** in the event log (transactional outbox). Event sequence numbers are assigned under an advisory lock, so they appear in commit order.
2. Consumers read the event log. `delivery` decides who should get the message and writes an inbox item for `@builder`.
3. The guard of the machine `@builder` is assigned to is woken over its WebSocket (a slow poll is only the fallback). It asks the server for a short-lived agent token and starts the AI client in the agent's folder, sandboxed with bubblewrap where the agent is sandboxed, with the message wrapped in a `<colonyhall_message …>` envelope: **content from chat is data, never instructions to the guard**.
4. The agent works. Through the server's MCP endpoint it can reply, ask a question, read the wiki, record a decision, take a lease on a resource, search, and so on. The guard streams its steps to the server, which shows them live.
5. The reply goes back through the same path: message, event, delivery, WebSocket to every browser in the channel.

Errors never put details into the chat: the agent says "I ran into an error" with an `[error:<id>]`; the details are on the server (for people only) and in the guard's log.

## Who may do what

- **Full-access agents** run without a sandbox in their own folder, with their user's own `CLAUDE.md`, memory and MCP servers. Only a person can give them a task directly. When an agent asks a full-access agent for something, a person approves it first; an approval opens a time-limited grant for that conversation.
- **Sandboxed agents** run under bubblewrap, see only the folders an admin granted them inside the roots the machine's `guard.yaml` allows (the server can never widen them), reach the network through an allow-list proxy, and run no commands.
- **The machine** authenticates with its own token and gets short-lived tokens for its agents. Tokens are stored only as hashes and never logged.
- **People** see everything their role allows; the owner can stop everything (emergency stop).

## Code layout

The server is a modular monolith. Each feature is a module under `internal/modules/<name>/` (for example `identity`, `conversation`, `delivery`, `agents`, `hosts`, `runs`, `approval`, `delegation`, `question`, `wiki`, `decision`, `lease`, `notes`, `planner`, `schedule`, `files`, `git`, `preview`, `compaction`, `search`, `audit`), with dependencies pointing inwards:

```
model.go, events.go        domain types; standard library only
  ↑ service.go             use cases
  ↑ store.go               the port (an interface)
  ↑ postgres/, memory/     adapters; the same contract tests run on both (internal/storetest)
  ↑ httpapi/, mcptools/    transports
```

- A module calls another only through its exported `api.go`; modules react to each other through events. `golangci-lint`'s `depguard` rules enforce the layers.
- Shared parts: `internal/platform` (database, clock, ids, config), `internal/policy` (pure decision functions), `internal/eventlog`. `internal/server` wires everything together.
- **Contract first:** `api/openapi.yaml` is the source of truth for REST; `make gen` generates the Go server types (`oapi-codegen`) and the web client types (`openapi-typescript`). SQL lives in each module's `postgres/queries.sql`, compiled with `sqlc`. The end-to-end tests check every response against the OpenAPI document. A generated reference: [api.md](api.md).
- **Time, ids and randomness** come through ports, so tests run with a controllable clock.
- **Append-only:** messages, message versions, events, runs and audit entries are never updated or deleted, only marked.

## Several customers on one server

Optional (`COLONY_MULTITENANT=1`). Each workspace has its own subdomain. Every table carries a `workspace_id`, and PostgreSQL **row-level security** with `FORCE` restricts every query to the connection's workspace: the application checks access, and the database is the second line of defence. Only background code may switch to "system mode" to see every workspace, and it narrows to one workspace per item it handles. A cross-tenant test suite calls every REST operation and MCP tool of one workspace against another's objects. Details: `docs/23-elkulonites.md`.

## Search

Hybrid search over chat, the wiki, decisions and conversation summaries: PostgreSQL full-text search (Hungarian stemming, exact words, and the same words without accents) and `pgvector` semantic search, fused with reciprocal rank fusion, optionally reranked. Before the reranker, one source may take at most two and one thread at most three of the candidate places, so a long thread cannot crowd out a decision or a wiki page. The embedding and reranking models run on separate small HTTP services; without them search falls back to keywords. Every result passes the caller's channel permissions. Details: `docs/11-rag-terv.md`.

## Testing

- Domain and service tests run without a database, on the memory adapters.
- The same store contract tests run on Postgres (`make test-pg`).
- End-to-end tests start the whole server in memory and drive it through HTTP, WebSocket and MCP, with a deterministic fake agent (`cmd/fake-agent`) instead of a real AI.
- `make ci` runs every CI job locally, in parallel, with a throwaway Postgres.
