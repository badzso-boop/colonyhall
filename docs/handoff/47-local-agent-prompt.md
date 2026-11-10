# Handoff prompt 47: a local coding agent with a deterministic harness

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/local-agent ../new-chat-worktrees/local-agent main
cd ../new-chat-worktrees/local-agent
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/47-local-agent-prompt.md)"
```

---

Branch: `feature/local-agent`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/local-agent`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push. Next free migration: check `migrations/`.

After each part: `make ci-quick` if round 42 is merged (otherwise `gofmt`, `make test`, `make lint`), `make test-pg` when a store changed, `make test-web` for PWA parts; `make ci` before the report. **Commit only when they pass.** Every fix needs a test that fails without it.

**This round has a design gate.** Part 0 ends with a design document and measurements, and **you stop there** until Norbi approves it in the chat.

**Depends on:** round 49 (the GPU services), round 39 (CodeGraph wired into the guard) and round 36 (the search and RAG database, the embedding model on the Raspberry Pi, hybrid search with reranking). If they are not merged when you start, design against their prompts and say in the report what you had to assume. **Do not touch the GPU machine's setup** (drivers, Ollama, models) without Norbi: write runbooks, measure only what he has installed.

## Why (Norbi, 2026-11-08)

Norbi is adding an **NVIDIA RTX 3060 12 GB** to his PC (WSL2, which can use the GPU) to run a **local coding model** (Ollama or similar), without Claude Code or Antigravity.

A 7–14B model in 12 GB is much weaker than Claude: short context, weaker tool use, it gets lost in long tasks. So the plan is **"a small model with a smart harness"**: the guard does everything that is not creative work, deterministically, and the model only writes the algorithm.

- The guard knows, per project, how to format, build, lint and test, runs them, and gives back **only the essence of the errors**.
- **Reuse before writing** (the idea of the Ponytail plugin) is enforced by the harness, **not by prompts**.
- Code search: CodeGraph for structure, plus **an embedding index of the code** (method and file names with a short description) in the RAG database, searched **hybrid** (keyword + vector + rerank).
- Documentation: **local DevDocs docsets first** (offline), well-known documentation sites through the guard's web-read as a fallback.
- **Everything as MCP servers**, usable by the local agent **and** by Claude and Antigravity agents.
- **Roles:** a strong agent (Claude, or the colleague's docs-only planner, round 38) **prepares precise tasks**; the local agent **does them**.

## Decisions (binding)

- The GPU goes into **Norbi's PC (WSL2)**.
- **Languages:** Colonyhall first (**Go, TypeScript/JavaScript/Node**), then **Python**, **Java** and **PHP**.
- **Norbi's decisions of 2026-11-09** (one symbol per task with a deterministic task splitter, an approved and saved project profile editable at runtime, a separate networked fetch step, context on demand without a fixed cap, limits that can be switched off, module-only final runs) are in `docs/19`, section 16. They are binding.
- **The code index is built on the PC** (the strong machine), with **the same embedding model and version as the Raspberry Pi** (round 36), so query-time embeddings from the Pi match.
- **Ponytail's logic, implemented deterministically** in the harness; nothing that only works because a prompt asks for it.
- **DevDocs (offline docsets) is the primary documentation source.**
- **MCP servers** for CodeGraph, the code and docs search (the embedding model and the RAG database), and the harness tools.
- **We do not write our own agent or model runtime** (decision, 2026-11-10; `docs/19` section 0, sources `docs/21`): the model runs in `llama.cpp` behind llama-swap (round 49); the agent loop is an **existing open-source coding agent** (candidates: little-coder/pi, OpenCode, Qwen Code) that the guard runs as a **new client**, like `claude` and `agy` (rule 7). What we write is the deterministic part: the task check, `run_task` with filtered errors, code and docs search, the reuse and duplicate checks, the gates after a run and the help request, as MCP servers and guard checks around the client.

## 0. Design gate

The detailed design draft is `docs/19-lokalis-agent-harness-terv.md` (written with Norbi; it is the source of truth for this round). Update it with your findings. Measure where you can, otherwise say "estimate". Answer:

0. **Choose the agent** (the first question of the gate): run little-coder (pi), OpenCode and Qwen Code with the same local model on 10–20 well-defined tasks from Norbi's repositories (in a sandbox; OpenCode auto-approves everything in non-interactive mode and read a hidden grader in a benchmark, `docs/21`), and compare: solved or not, tool-call failures, steps, wall time, how each can be driven non-interactively with a streamed, machine-readable log (for the live steps), how MCP servers are given to it, and its licence. Propose one, with the numbers.
1. **Model runtime and models on 12 GB:**
   - Ollama vs `llama.cpp` server vs others; constrained output (JSON schema or grammar) for reliable tool calls; the OpenAI-compatible tool-calling API as the one adapter (rule 7);
   - candidates (e.g. Qwen2.5-Coder 7B and 14B, Qwen3 14B, DeepSeek-Coder-V2-Lite), quantization, the context that fits next to the weights, tokens per second, **tool-call reliability** on a small fixed set of prompts;
   - one GPU: one local run at a time, the queue, model keep-alive, VRAM in the host report.
2. **The agent loop in the guard** (a new client, `local`): steps, budgets (steps, tokens, wall time), how the context is kept small (only relevant snippets, summaries of earlier steps, never whole files), when it gives up, what it reports.
3. **The task format a planner hands over** (a schema, not prose):
   - goal, target files and symbols, the signature to implement, acceptance (tests that must pass, or new tests to write first), constraints (no new dependency, style);
   - how a planner creates one (an MCP tool, e.g. `local_task`), how it reaches the local agent (delegation to a sandboxed agent already needs no approval), and what comes back (the diff, the gate results, a short note).
4. **Project task profiles** (deterministic commands):
   - auto-detection per language (Go: `go.mod`; Node: `package.json` scripts; Python: `pyproject.toml`, `requirements.txt`, `pytest`, `ruff`, `mypy`; Java: Maven or Gradle);
   - an optional per-repo override file;
   - the commands the model may ask for (`format`, `build`, `lint`, `test` with a target); never a free shell.
5. **Error filters** per tool, from machine-readable output where it exists:
   - Go: `go build`, `go vet`, `go test -json`, golangci-lint JSON;
   - TypeScript: `tsc`; ESLint JSON; Vitest and Jest JSON;
   - Python: `ruff` JSON, `mypy`, `pytest` JUnit XML;
   - Java: `javac`, Maven or Gradle with the Surefire XML reports.

   The output is a **structured list**: file, line, column, rule or test name, message, a few lines of code around it. Deduplicated, capped, and the same shape for every language.
6. **Targeted tests:** CodeGraph's `affected` (or the language's own test selection) instead of the whole suite; when a full run is still needed.
7. **Edits:** structured edit tools (replace a symbol's body, insert a function, apply a small patch) validated at once (format, then build) instead of free file writes; rollback on a broken result.
8. **Reuse before writing (Ponytail, deterministic):**
   - **before** the model writes: from the task's signature and keywords, the harness searches CodeGraph, the code index (hybrid) and the project's dependencies and standard library docs, and puts the best few candidates (with their signatures and one-line descriptions) into the prompt;
   - **after** an edit: the harness checks whether a new function is a near-duplicate of an existing one (name, signature, normalised body, embedding similarity) and **rejects** it with "this already exists in X";
   - a new dependency is refused unless the task allows it.
   - Look at Ponytail's licence before reading its code; take the ideas, write our own code.
9. **The code index:**
   - what is embedded (each function, method, type: its qualified name, file, signature, and a **short description**: the doc comment when there is one, otherwise a one-sentence summary generated **on the PC by the local model** at index time, cached by content hash); file-level entries;
   - built with CodeGraph's symbol boundaries;
   - stored in the RAG database (round 36) as a new source kind with **access control by machine and folder grant** (an agent sees only code it may read);
   - incremental updates from git changes;
   - the bulk build on the PC, queries embedded on the Pi.
10. **Docs:**
    - DevDocs docsets downloaded on the PC (which ones, size, updates), searched offline;
    - an allowlist of documentation sites for the web-read fallback;
    - whether docs also go into the RAG database for hybrid search.
11. **The MCP servers** (names, tools, who gets them):
    - CodeGraph (round 39, as it is);
    - **code search** (hybrid over the code index);
    - **docs search**;
    - **harness tools** (`run_task` with filtered errors, structured edits, the reuse check);
    - **`local_task`** for planners.

    The harness tools are just as useful to Claude agents: filtered errors save their context too. Sandboxed agents reach them through the guard's bridge.
12. **Security:** the local agent runs sandboxed, with no network except the guard's bridges; the model server listens on localhost only; the commands come only from the task profile; nothing from the code or the docs is treated as an instruction.
13. **Evaluation:**
    - a fixed set of about 20 well-defined tasks from Norbi's repositories, with known good solutions and tests (kept out of the repository if private);
    - **metrics:** solved yes or no, steps, wall time, tokens, rejected duplicates;
    - compare models, quantizations and harness options, and the local agent against a Claude agent on the same tasks.

The GPU itself (driver, CUDA in WSL2, the model runtimes, the guard's GPU relay and scheduler) is prepared by round 49 (`docs/handoff/49-gpu-prompt.md`, runbook `docs/20-gpu-beallitasa.md`); build on it. **Then stop** and tell Norbi in the chat what you propose and what he has to decide.

## Parts after the gate (to be confirmed by the design)

1. The chosen agent as a new client in the guard (adapter, sandbox, streamed steps, the MCP servers wired in; a fake model server in tests: scripted tool calls).
2. Task profiles and error filters for Go and TypeScript/Node, with golden tests on real tool output.
3. Structured edits with validation and rollback.
4. The code index (build on the PC, incremental updates, access control) and the code-search MCP server.
5. Reuse before writing: the candidate search and the duplicate check.
6. DevDocs and the docs-search MCP server; the web-read fallback list.
7. `local_task` and the hand-over from planners; the result back in the thread.
8. Python, Java and PHP profiles and filters.
9. The evaluation set and a report command; the first numbers.
10. PWA: the local agent in the agent wizard (round 33), its queue and GPU state on the Machines page.

## Out of scope

Fine-tuning a model; more than one GPU; running the model on the homelab; replacing Claude for planning.

## Docs

Hungarian: `docs/01-terv.md` (the planner and local coder model), `docs/02-muszaki-specifikacio.md` (the client, the MCP servers, the task format, the index), `docs/06-allapot-es-uzemeltetes.md` (the GPU, the queue, what to check), `docs/19-lokalis-agent-harness-terv.md`, `task.md`.

## Report

`docs/handoff/47-local-agent-report.md` (Hungarian): the commits; the part 0 measurements (models, speed, tool-call reliability); the evaluation numbers; what is deterministic and what still relies on the model; the deploy order; what the reviewer should look at first (the command allowlist, the sandbox bridges, the access control of the code index).
