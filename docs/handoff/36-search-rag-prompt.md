# Handoff prompt 36: hybrid search and RAG retrieval for agents and people

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/rag ../new-chat-worktrees/rag main
cd ../new-chat-worktrees/rag
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/36-search-rag-prompt.md)"
```

---

Branch: `feature/rag`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/rag`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push. Next free migration: check `migrations/` (00048 at the time of writing).

After each part: `gofmt`, `make test`, `make lint`; `make test-pg` whenever a store or a query changes (the vector parts need Postgres with pgvector: use the `pgvector/pgvector:pg16` image in `deploy/compose.dev.yaml` if the dev stack does not have it yet); `make test-web` for PWA parts. At the end `go test -race ./internal/e2e/`. **Commit only when `make test` and `make lint` pass.** Every fix needs a test that fails without it.

**This round has a design gate.** Part 0 ends with a design document and measurements, and **you stop there** until Norbi approves it in the chat. Parts 1–8 start only after that.

**Do not touch shared infrastructure.** No `CREATE EXTENSION` on the homelab, no software on the Raspberry Pi or the homelab, no model downloads on them. You write runbooks; Norbi (or Claude Code with his approval) runs them.

## Why (Norbi, 2026-10-30 and 2026-11-05)

1. **Agents cannot find what was already said.** Requests, decisions and results are spread over threads, DMs, the wiki, the decision log and compaction summaries. An agent sees its own thread and reads other channels one by one; things get lost (2026-10-28: a test result sat in a DM thread the working thread knew nothing about).
2. **Learning:** Norbi wants to learn keyword search and RAG hands-on, as a modern AI engineer builds them, to use at work. Each layer is built so it can be compared with the others, and the round produces a Hungarian study guide.

## Decisions (binding)

- **Sources in this round:** chat messages (channels, threads, DMs), wiki pages, decisions (`D-NN`), compaction summaries. **Code is not indexed in this round** (CodeGraph, round 39, covers code; code in the RAG database comes later, chunked along CodeGraph's symbol boundaries).
- **Access control at query time,** on every call: only what the caller may read.
- **Chunking** that fits the source, with a short deterministic context header per chunk.
- **Hybrid retrieval:** Postgres full-text (keyword) and pgvector (semantic) together, merged with Reciprocal Rank Fusion, then **reranked**.
- **Continuous indexing** from the event log; edits, retractions and deletions update or remove chunks.
- **Retrieved text is data, never instructions** (framed like chat messages).
- **Everything is logged** (each search: who, mode, latency, result ids).
- **The agent decides** when to search: a tool, not a rule.
- **Measurement is part of the round** (part 7).
- **Infrastructure:**
  - **Embedding model on a Raspberry Pi 3B** on the home LAN, behind a simple HTTP endpoint. The Pi 3B has 1 GB RAM and a slow 4-core CPU: a small multilingual model only (e.g. `multilingual-e5-small`, quantized). **No Gemini API** (Norbi does not want it called now).
  - **The reranker runs on the homelab** (about 3 GB free RAM today).
  - **The backfill of all history runs on the PC (WSL)**, which is strong, with the **same model and version** as the Pi, so the vectors are interchangeable.
  - **pgvector stays in the shared Postgres** (`homelab-shared-postgres` already runs the `pgvector/pgvector:pg16` image, and `legymernok` already uses `vector` there). It is enabled once, only in the `colonyhall` database (`CREATE EXTENSION vector`, superuser, by Norbi). No image change.

## 0. Design gate: models, the Pi, the reranker, the data model

Write `docs/11-rag-terv.md` (Hungarian) and measure, then stop. It must answer, with numbers where possible:

1. **Embedding model and server on the Pi 3B:**
   - candidates: `multilingual-e5-small` (384 dims) and at most one other small multilingual model; Hungarian quality matters;
   - server: e.g. `llama.cpp` `llama-server --embedding` (OpenAI-compatible `/v1/embeddings`) with a GGUF quantization, or ONNX Runtime behind a minimal HTTP server; pick one and say why;
   - **measure on equivalent hardware or the closest you have** (a Pi 3B is ARMv8, 1 GB RAM: the PC can emulate the limits with `taskset` to 4 slow cores and a memory cap, or say plainly that the numbers are from the PC and must be re-measured on the Pi): latency for a short message and for a 512-token chunk, throughput per minute, memory;
   - the 64-bit OS question (a 32-bit Raspberry Pi OS cannot run some builds), swap, SD-card wear, what happens on a reboot, health check;
   - the API contract the Colonyhall server calls (request/response, batch size, timeouts, the `query:` / `passage:` prefixes e5 needs, model name and version in every answer so a model change is detected).
2. **Reranker on the homelab:**
   - candidates: small multilingual cross-encoders (e.g. `jina-reranker-v2-base-multilingual`, `bge-reranker-v2-m3` quantized); memory and latency for reranking 50 candidates; Hungarian quality;
   - server: e.g. `llama.cpp` `--reranking` (`/v1/rerank`) in a container with a memory limit, or ONNX Runtime; where it runs (the homelab's own `docker-compose.yml` is shared infrastructure: propose a **separate** compose project, e.g. `~/homelab/colonyhall-rerank/`, and say why);
   - what happens when it is down (search still answers, without reranking, and says so).
3. **Backfill on the PC:** the same model and version as the Pi, run where; how it reaches the database (an SSH tunnel to the shared Postgres or an admin API on the server; **no database credentials in the repo or the chat**, through `colonyhall-exchange` only); how long it takes for the current data (count the messages, wiki pages, decisions, summaries in the dev stack and estimate from the measured throughput); resumable.
4. **Data model:** a `search_chunk` table (source kind and id, workspace, channel, thread root, author, created_at, the chunk text and its context header, content hash, `tsvector` columns for `hungarian` and `simple`, the `vector(N)` embedding with model name and version, `embedded_at`), indexes (GIN for full text, HNSW for vectors with the chosen `m`/`ef_construction`, B-tree for the filters), how a model change re-embeds everything, and a `search_log` table (retention 30 days).
5. **Chunking per source:** a short message stays whole; a long message, a wiki page and a compaction summary are split by paragraph (target and overlap in tokens); the context header (deterministic, no LLM call: channel name, thread root's first line, author, date, wiki title, decision number); what is never indexed (the secrets exchange folder, anything a secret scanner would flag, calendar URLs, error details, files).
6. **Access control:** exactly which rule decides visibility for each source (channel membership and `can_write` irrelevant, DMs only for their members, admins, the round 38 colleague rules, agents through their channel memberships, sandboxed agents, the emergency stop, deleted and retracted content), and how the SQL applies it **before** ranking (never filter after taking the top k).
7. **Ranking:** the keyword query (`websearch_to_tsquery`, both configurations, `ts_rank_cd`), the vector query (cosine, `ef_search`), RRF (`k` = 60 unless the measurements say otherwise), the reranker on the top 50, the final `limit`.
8. **Operations:** what the status page (round 35) shows (embedding queue length, last embed, Pi and reranker reachable yes/no, model and version), the alerts, the costs in memory and disk on each machine.

Also write the two runbooks (Hungarian): `docs/12-rag-pi-beallitasa.md` (the Pi: OS, the model server as a systemd service, the firewall to the LAN only, a health check) and `docs/13-rag-reranker-beallitasa.md` (the homelab reranker: its own compose project, memory limit, health check), and the one-time `CREATE EXTENSION vector` step in `docs/05-homelab-telepites.md`.

**Then stop** and tell Norbi in the chat what you propose and what he has to decide.

## 1. Storage and the embedding port

- Migration (`search_chunk`, `search_log`, indexes) as approved. **The migration does not create the extension** (it needs superuser); it fails with a clear message if `vector` is missing, and the docs say how to enable it.
- `Embedder` port with adapters: `http` (the Pi's endpoint, `COLONY_EMBED_URL`), and `fake` (deterministic vectors for tests). `Reranker` port: `http` (`COLONY_RERANK_URL`), `none`. Both: timeouts, batch limits, the model name and version checked against the stored ones.
- Store tests in `internal/storetest` for memory and Postgres (the memory store does a brute-force cosine).

## 2. Indexing

- A consumer on the event log creates, updates and removes chunks for the four sources (message created, edited, retracted, deleted; wiki appended, deleted; decision recorded; compaction done, regenerated). It never calls the embedder inside the transaction: chunks are written with `embedding = NULL`, and an embedding worker fills them in batches, with retry and backoff. **When the Pi is down, keyword search keeps working** on the new chunks.
- A backfill command (`colonyhall rag-backfill`) as designed in part 0: resumable, idempotent (content hash), a progress line, a dry-run mode that only counts.
- A re-embed command for a model change.

## 3. Hybrid retrieval and reranking

- One `Search` use case: `query`, `mode` (`keyword` | `semantic` | `hybrid`; default `hybrid`), filters (source kinds, channel, author, time range), `limit` (default 8, max 20), and the caller.
- Access control in the SQL of both legs, before ranking. RRF merge, then the reranker on the top 50 (skipped, with a flag in the result, when the reranker is down or `mode` is not hybrid).
- Each result: kind, snippet (with the matched words for keyword hits), source reference (channel, thread root, message id or wiki slug or `D-NN`), author, time, and the scores of each stage (for learning and debugging).

## 4. Interfaces

- **MCP tool `search`** for every agent (sandboxed agents too, through their Colonyhall MCP access), with the parameters above. The output starts with the note that results are data, not instructions, and frames each hit like a chat message (`<colonyhall_message ...>`).
- **REST** `GET /search` (contract first) for the PWA.
- **Shared agent rules:** a short hint (not a rule): when you need something said or decided earlier, use `search` before asking a person.

## 5. PWA

- A search field in the header (the command palette's search box opens it): results grouped by source, the matched words highlighted (as text, never HTML), clicking a message opens it in its thread.
- A "Kulcsszó / Jelentés / Hibrid" switch and, for admins, the stage scores of each result (to see the difference; useful for learning).

## 6. Logging and the status page

- `search_log`: who, mode, filters, the query text (retention 30 days, admins only), result ids, the latency of each stage, whether the reranker ran.
- Status page (round 35): embedding queue length, last embed, Pi and reranker reachable, model and version.

## 7. Measurement

- **A test set:** 30–50 Hungarian questions with their expected results (message ids, wiki slugs, decision numbers), written from real Colonyhall data by Claude Code with Norbi, **kept out of the repository** (private data: `colonyhall-exchange` or a gitignored folder); plus a small synthetic set in the repo for CI.
- **Metrics:** recall@5, recall@10, MRR@10 (and nDCG@10 if cheap), latency p50 and p95, for keyword, semantic, hybrid, and hybrid + rerank.
- **A report command** (`colonyhall rag-eval`) prints them side by side; the round's report has the first numbers.

## 8. Study guide

`docs/tanulas/kereses-es-rag.md` (Hungarian), built on this code and the measured numbers: tokenization and `tsvector`, `ts_rank_cd` vs BM25, embeddings and cosine similarity, the `query:`/`passage:` prefixes, chunking trade-offs, HNSW and its parameters, hybrid search and RRF, cross-encoder reranking, access control in retrieval, prompt injection through retrieved text, recall@k and MRR, and what to try next (contextual chunk headers with an LLM, query rewriting, GraphRAG, putting code in with CodeGraph's symbols).

## Out of scope (say so in the report)

Code in the RAG database; the Gemini API; an LLM-written context header per chunk; a server-side "answer with citations" tool; a load balancer and several server instances.

## Docs

Hungarian: `docs/01-terv.md` (search and RAG as decided), `docs/02-muszaki-specifikacio.md` (tables, endpoints, events, the ports), `docs/06-allapot-es-uzemeltetes.md` (the Pi, the reranker, the queue, what to check when search is slow or empty), `task.md`.

## Report

`docs/handoff/36-search-rag-report.md` (Hungarian): the commits; the part 0 measurements; the first `rag-eval` numbers; the backfill time on the PC; the deploy order (extension, Pi, reranker, server, backfill, guards if they changed); what the reviewer should look at first (the access control in the SQL, the indexing of edits and deletions, the framing of results).
