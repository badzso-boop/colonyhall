# Handoff prompt 08: cost limits — session rotation and daily token budget

```sh
cd /mnt/g/Projects/new-chat && git switch main
agy --model claude-opus-4-6-thinking -i "$(cat docs/handoff/00-ground-rules.md docs/handoff/08-cost-limits-prompt.md)"
```

---

Branch: `feature/cost-limits`. Spec: `docs/01-terv.md` → "A következő kör tervei", item 2. Read `internal/modules/runs/` (the run log and `/usage`), `internal/modules/hosts/` (assignment), `internal/guard/direct.go` (sessions, state file).

## 1. Session rotation

- Assignment field `max_session_tokens` (int, default 150 000, 0 = no limit), contract-first, migration, hosts API, dev page assign form.
- The guard records per thread session the tokens of its **last** run (input + cache read + cache write — the context size) in the direct state file. When that reached `max_session_tokens`, the next run in the thread starts a **new** session: a new id (`UUIDv5(agent, thread + "#" + generation)`, keep the generation in the state; for Antigravity: no `--conversation`), and the prompt gets a short recap: the thread's last 3 messages (via `GET /api/v1/messages/{id}/thread`, truncated like the compact mode's context). The guard posts one line in the thread: "🔄 Started a fresh session (the previous one reached N tokens)."
- Tests: rotation threshold, new id, recap content, state persistence.

## 2. Daily token budget

- Assignment field `daily_token_limit` (int64, 0 = none).
- Server: `GET /api/v1/agent/budget` (agent token) → `{limit, used_today, remaining, resets_at}` computed from the run log for the current Budapest day. Humans see the same per agent in the 📊 panel and can change the limit in the "Gépek" panel.
- Guard, before each run (decide, work, direct): ask the budget; if `limit > 0` and `remaining <= 0`: do not run, **do not ack** (items stay in the inbox), post once per agent per day in the thread "⏳ My daily token budget is used up; I continue after <resets_at local time>." and skip until `resets_at` (no polling storm: wait until then or until the limit changes in the host config). Halt checks unchanged.
- Tests (scripted adapter, manual clock): over budget → no run, items stay, one note per day; next day → runs again; raised limit → runs again; budget endpoint agent-only and correct across the midnight boundary.

## Docs

`docs/02-muszaki-specifikacio.md` (Őr: rotation, budget; endpoints), plan section "Keretek és költségkorlátok": mark what is implemented. `CLAUDE.md` schedule. Hungarian.
