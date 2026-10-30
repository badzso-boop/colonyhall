# Handoff prompt 36 (DRAFT, idea only): search and RAG search

> **Status: idea, not ready for implementation.** Norbi and Claude Code will design this in detail before any work starts (data model, embedding provider, infrastructure, evaluation). **Do not start coding from this file.** When the design is agreed, this draft is replaced by a real handoff prompt with the usual branch, worktree and test rules.

## Why (Norbi, 2026-10-30)

1. **Agents cannot find what was already said.** Requests, decisions and results are spread over threads, DMs, the wiki, the decision log, thread notes and compaction summaries. An agent sees its own thread, and reads other channels one by one with `read`. That is slow and costly, and things get lost: on 2026-10-28 a test result sat in a DM thread that the working thread knew nothing about.
2. **Learning:** Norbi wants to learn both keyword search and RAG (retrieval-augmented generation) hands-on, to use them later at work. So both are built, as separate, comparable layers, with a way to measure how well each works.

## The idea (Claude Code's proposal, to be discussed)

### Layer 1: full-text search (keyword)

- **What it searches:**
  - messages (not retracted ones);
  - wiki pages;
  - decisions (`D-NN`);
  - compaction summaries;
  - later perhaps thread notes and the text of uploaded files.
- **How:** PostgreSQL full-text search.
  - A `tsvector` column (or generated column) per source, with a GIN index.
  - The `hungarian` snowball configuration for stemming, plus `simple` for names, handles and identifiers.
  - Ranking with `ts_rank_cd`, highlighted snippets with `ts_headline`.
  - Optionally `pg_trgm` for typos and partial words.
- **Why first:** no new service, no external API, nothing leaves the server. It is also the baseline the semantic search is measured against.

### Layer 2: semantic search (RAG retrieval)

- **Embeddings:** the text is split into chunks, and each chunk becomes a vector.
  - Long messages, wiki pages and summaries are cut by paragraph, with a little overlap.
  - Short messages stay whole, with some thread context added.
- **Embedding provider as a port with adapters** (architecture rule 7), so providers can be compared. Candidates:
  - a local model on the homelab through Ollama (e.g. `bge-m3` or `multilingual-e5`: good for Hungarian, free, CPU cost);
  - the Gemini embedding API (needs an API key; text leaves the house).
- **Storage:** `pgvector` with an HNSW index. Every vector records its model name and version, so a model change can re-embed everything.
  - **Open:** the shared `homelab-shared-postgres` also serves other projects. Is the extension added there, or does Colonyhall get its own Postgres container (or a separate vector store)?
- **Indexing:**
  - asynchronous, from the event log (`message.created`, wiki and decision events, compaction), through a job queue (River is already in the stack);
  - a backfill command for existing data;
  - a retracted or deleted item is removed from the index too.
- **Hybrid retrieval:** run the keyword and the vector search together and merge them with Reciprocal Rank Fusion. Optionally add a reranker step later.
- **"Generation" in RAG:** the agents themselves are the LLMs. The server only retrieves; the agent gets the passages with their sources (channel, thread, message id, link) and answers from them. A server-side "answer with citations" tool can come later.

### Access rules (the most important part)

- **Every result is filtered by what the caller may read**, at query time and on every call, never only at indexing time:
  - channel membership;
  - DMs only for their members (and admins);
  - human-only content (files, error details) excluded for agents;
  - `deleted_at` respected.
- **Results are data, never instructions:** the MCP output starts with that note, as `planner_read` does.
- **Never indexed:**
  - the secrets exchange folder;
  - tokens and anything a secret scanner would flag;
  - the planner calendar URLs.

### Interfaces

- **MCP tool `search`:** parameters `query`, `mode` (`keyword` | `semantic` | `hybrid`, default `hybrid`), optional filters (channel, author, kind, time range) and `limit`. Each result has its kind, a snippet, the source, its time and a score. Sandboxed agents get it through the guard-side read-only server, like `planner_read` (round 29).
- **REST:** for the PWA search box and the dev page.
- **UI:**
  - a search field with results grouped by source;
  - clicking a result opens the message in its thread;
  - a "keyword vs semantic" toggle to see the difference (useful for learning).

### Measuring it (for learning and for decisions)

- **Test set:** a small, hand-made set of queries with the expected results, e.g. "how did we decide about the calendar link?" → the decision and the thread.
- **Metrics:** recall@k and MRR for keyword, semantic and hybrid search; a report command prints them side by side.
- **Comparison:** it decides with numbers which embedding model and which chunking is better for Hungarian chat text.

### Suggested order (each step can be learned and tested on its own)

1. Keyword search over messages, with the access filter and the `search` tool (`mode=keyword`).
2. The other sources (wiki, decisions, summaries) and the UI.
3. The embedding port with one adapter, chunking, indexing from events, backfill, `pgvector`.
4. Hybrid search (RRF), the test set and metrics.
5. A second embedding adapter and the comparison; optionally a reranker.

## Open questions for the design discussion

- **Embedding provider:** local (Ollama on the homelab: how much CPU/RAM can it use?) or the Gemini API (key, cost, data leaving the house)? Or both, compared?
- **Postgres:** `pgvector` on the shared Postgres, or a separate Colonyhall Postgres or vector store?
- **Scope:** which sources are indexed, and from when (all history or only new data)?
- **Agent defaults:** should agents search on their own (a rule in the shared agent rules: "search before asking"), or only when asked?
- **Learning materials:** should each round's report also explain the concepts (tsvector, BM25 vs `ts_rank`, embeddings, HNSW, RRF, recall@k) in Hungarian, as a short study guide?
- **Order:** this comes after the PWA rounds (31–35), or in parallel on its own branch?
