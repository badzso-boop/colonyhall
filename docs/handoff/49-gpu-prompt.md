# Handoff prompt 49: the GPU machine — runtimes, the guard's GPU relay and the VRAM scheduler

```sh
cd /mnt/g/Projects/new-chat && git switch main && git pull --ff-only
git worktree add -b feature/gpu ../new-chat-worktrees/gpu main
cd ../new-chat-worktrees/gpu
claude "$(cat docs/handoff/00-ground-rules.md docs/handoff/49-gpu-prompt.md)"
```

---

Branch: `feature/gpu`, **in its own git worktree** at `/mnt/g/Projects/new-chat-worktrees/gpu`. This overrides the ground rules' "git switch -c" step: never work in `/mnt/g/Projects/new-chat` itself, never touch `main`, never merge, never push.

After each part: `make ci-quick` if round 42 is merged (otherwise `gofmt`, `make test`, `make lint`); `make test-web` for PWA parts; `make ci` before the report. **Commit only when they pass.** Every fix needs a test that fails without it.

**This round has a design gate** (part 0). You stop there until Norbi approves it in the chat.

**Do not install anything on Norbi's PC yourself** (drivers, CUDA, Ollama, `llama.cpp`, models, services). Write the runbook; Norbi runs it (or Claude Code with his approval, step by step). You may **read** the state (`nvidia-smi`, `ollama list`, service status) and **measure** with what is installed, with his permission.

## Why (Norbi, 2026-11-08)

Norbi puts an **NVIDIA RTX 3060 12 GB** (ASUS Dual V2) into his PC and uses it from **WSL2**. Three later rounds need it:

- **36b** (the code index): a code embedding model and a code reranker, at index time and at query time;
- **47** (the local coding agent): a 7–14B coding model with tool calling, and one-sentence symbol descriptions at index time;
- the **history backfill** of round 36a's text embeddings (the Raspberry Pi's model, run on the PC for speed).

They share **one 12 GB card**, and **the PC is not always on**. This round makes the GPU a well-behaved, observable service of the guard, so the later rounds only plug into it.

## Decisions (binding)

- The GPU is used from **WSL2** on Norbi's PC (the `pc-wsl` guard).
- **Only the Windows NVIDIA driver** is installed; no Linux display driver inside WSL (WSL2 exposes the GPU through the Windows driver).
- **The server never connects to the PC directly.** GPU work goes **through the PC guard's existing host connection** (a relay like round 20's file-system requests): no new port on the PC, nothing exposed to the LAN or Tailscale.
- **When the PC is off,** every GPU-backed call fails **fast** with a **short, meaningful error** (e.g. `GPU unavailable: the GPU machine (pc-wsl) is offline`), the server shows the state on the status page, and every agent and machine gets the same answer.
- The model runtimes listen on **localhost only**, inside WSL.

## 0. Design gate

Write the runbook `docs/20-gpu-beallitasa.md` (Hungarian) and the design section in it, measure what Norbi has installed, then stop. Answer:

1. **Hardware and Windows** (for Norbi, as a checklist): power supply (the card draws about 170 W, one 8-pin PCIe connector, NVIDIA recommends a 550 W supply), the Windows driver, WSL2 version, `nvidia-smi` inside WSL, `.wslconfig` (memory for WSL, since models also use system RAM when loading), keeping WSL running (the keepalive of rounds 38/41 if the guard should serve while no terminal is open).
2. **Runtime (decision, 2026-11-10: `llama.cpp` behind llama-swap, no Ollama; `docs/19` section 0, `docs/21`):**
   - **`llama-server`** (`llama.cpp`) with CUDA, for generation, embeddings and reranking (`--reranking`, `/v1/rerank`), with grammar or JSON-schema constrained output; thinking models with `--reasoning-format deepseek` so the reasoning comes in `reasoning_content`, apart from the answer; streaming on;
   - **llama-swap** in front of it as **one** systemd service in WSL, listening on `127.0.0.1` only: the model list (file, quantization, context, flags) in its config, models loaded on the first request and swapped or unloaded after an idle time. The guard does **not** start model processes itself;
   - how `llama.cpp` is built (prebuilt CUDA release or a build with the CUDA toolkit for WSL), the model files (GGUF) on the WSL ext4 disk (not under `/mnt/*`), each pinned by URL and sha256; versions pinned and recorded; how to upgrade;
   - Ollama (already installed on the PC) is not used by Colonyhall; Norbi may keep it, or LM Studio, for his own experiments.
3. **Models** (candidates only; rounds 36b and 47 choose with their measurements): a coding model (e.g. Qwen2.5-Coder 7B/14B, Qwen3 14B), a code embedding model (e.g. `jina-embeddings-v2-base-code`, `nomic-embed-code`, `bge-m3`), a code reranker, the Pi's text model for the backfill. For each: VRAM at the planned quantization and context, load time, what fits together on 12 GB.
4. **The VRAM scheduler** (in the guard, on top of llama-swap: llama-swap loads and unloads, the guard decides **what may run when**): one card, several kinds of work. Propose and justify:
   - **priorities:** interactive queries (a code search's embedding and rerank: seconds) above the local agent's generation (minutes) above bulk indexing and backfills (hours, preemptible);
   - **what stays loaded** (e.g. the small embedding model always, the coding model on demand with a keep-alive), what is unloaded when, expressed as llama-swap config (groups, idle times) rather than guard code where it can be;
   - **model swaps** (decision, 2026-11-10): round 47 may use one model to scout and another to write; a swap (about 15 s on one card) is allowed, the request waits, and the PWA shows it ("model swap: A → B") in the run's live progress and on the GPU card;
   - **a queue** with limits, timeouts, cancellation; bulk jobs pause while interactive work waits;
   - **out-of-memory** handling: detect, unload, retry once, report;
   - **other programs on the card** (decision, 2026-11-10): the card also drives Norbi's monitors and his games (about 1.6 GB for the Windows desktop alone, more while gaming). The scheduler reads the VRAM used by others (`nvidia-smi`), plans with what is really free, and when too little is left, local work waits (or uses a smaller model if the task allows it) instead of failing or stalling a game; the GPU card on the status page shows it ("busy: other programs use 9.8 GB").
5. **The GPU relay:** the frames between server and guard (request id, kind: `embed` / `rerank` / `generate`, model, input; streaming for generation), size limits, timeouts, auth (only the server over the host connection; nothing from agents directly except through the guard's own tools), and what an offline guard returns.
6. **What the guard reports:** in the host report (contract first): GPU name, VRAM total and used, temperature, utilization, the runtimes' health and versions, the loaded models, the queue length per priority. The Machines page and the status page (round 35) show it; the status page gets a "GPU" card.

**Then stop** and tell Norbi in the chat what he has to do on the hardware and Windows side, and what you propose.

## Parts after the gate

1. **GPU detection and reporting** in the guard (`nvidia-smi --query-gpu=... --format=csv`, parsed; tests with recorded output; works without a GPU: reports none).
2. **Runtime health checks** for llama-swap and its `llama-server` models (`guard.yaml`: its URL on localhost; the models it serves and which are loaded); reported.
3. **The GPU relay** (server ↔ guard), the server-side `GPU` port the later rounds use (`Embed`, `Rerank`, `Generate`), and **the fast, short error when the GPU machine is offline** (tests: offline guard, a slow runtime, an oversized request).
4. **The VRAM scheduler** in the guard as designed (tests with fake runtimes: priorities, preemption of bulk work, out-of-memory retry, cancellation).
5. **Status and Machines pages:** the GPU card and the per-machine GPU details.
6. **A smoke command:** `colony gpu check` (or part of `colony doctor`, round 41): driver, runtimes, a tiny embed, a tiny rerank, a few generated tokens, with timings.

## Out of scope

Choosing the final models (rounds 36b and 47 measure and choose); the local agent's loop (47); the code index (36b); more than one GPU; GPU on any other machine.

## Docs

Hungarian: `docs/20-gpu-beallitasa.md` (the runbook and the design), `docs/02-muszaki-specifikacio.md` (the relay, the port, the report fields), `docs/06-allapot-es-uzemeltetes.md` (what to check when the GPU card says offline or busy), `task.md`.

## Report

`docs/handoff/49-gpu-report.md` (Hungarian): the commits; what Norbi installed and the measured VRAM, load times and speeds per candidate model; the scheduler's design and its tests; the deploy order (server for the report fields and the relay, then the PC guard); what the reviewer should look at first (the relay's auth and size limits, the offline error path, the scheduler's preemption).
