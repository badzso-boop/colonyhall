# Handoff prompt 46 (DRAFT): GDPR, anonymising a person, data export

> **Status: draft for information only. Do not start from this file.** Norbi and Claude Code discuss and work it out in detail first (decided 2026-11-07). When the design is agreed, this draft is replaced by a real handoff prompt. **The legal side needs a lawyer or a data protection expert; this draft is technical.**

## Why (Norbi, 2026-11-07)

As soon as Colonyhall handles other people's data (a colleague, later customers), the EU's GDPR applies: a person may ask to see their data, to take it with them, and to have it erased. Erasing conflicts with "nothing is ever deleted" (rule 4) and with the audit hash chain.

Norbi's idea: a deleted user becomes **"Törölt felhasználó"** (or "anonymous") **everywhere**, so the conversations stay readable. He is worried that rewriting every row is a heavy database operation, and asks what the industry does.

## What the industry does (to discuss)

- **Do not rewrite every row: anonymise the identity once.**
  - Messages, events, runs and audit entries refer to a person by `actor_id`; the name, handle, avatar, e-mail live in one place (the actor). Anonymising that one record (a "Törölt felhasználó #4821" display name, a random handle, no avatar, no e-mail, no password, all tokens and sessions revoked) makes every message show it automatically.
  - This is cheap (one row) and keeps the threads readable. Slack, Mattermost and Discord work this way for deactivated or deleted accounts.
- **What still needs looking at:**
  - **free text** that names the person: `@handle` mentions in other people's messages, quotes, wiki text, decision text, compaction summaries. Options: leave them (other people's own words), rewrite only the mention tokens (they are structured), or a full-text redaction pass on request. A legal question as much as a technical one;
  - **the person's own messages:** keep them under the anonymous name (the usual default for work chat, because removing them breaks other people's threads), or delete or blank their content on request;
  - **files** they uploaded, their notes, their personal tasks and calendar (round 28 planner), search index chunks (round 36), archived data (round 44), backups.
- **Append-only and the audit chain:**
  - the common technique for event-sourced and append-only systems is **crypto-shredding**: personal data in events and audit entries is stored encrypted with a per-person key; erasing the person deletes the key, the rows stay, the chain stays valid, and the data can no longer be read;
  - or keep personal data out of the chained payloads entirely (only `actor_id`), so anonymising the actor is enough.
- **Backups:** erased data still sits in old backups until they expire. The usual answer: a documented backup retention, and a list of erasures that is re-applied after any restore.
- **Export (right of access and portability):** a ZIP with the person's profile, their messages, files, notes and tasks as JSON (and readable Markdown), generated in the background, downloadable once.

## Open questions to settle before the real prompt

- Who may erase or export a person: the owner on request, the person themselves, both?
- Is the default "keep my messages under an anonymous name" or "remove my messages"? Per workspace setting?
- Agents: an agent is not a person, but its runs and logs contain what people wrote to it. Retention of run logs and steps (and round 44's archive).
- Retention periods for each kind of data (and the privacy policy that states them), a record of processing activities, the data processing agreement for future customers (round 45).
- Today's data: anything to clean up before the colleague starts?

## Rough parts (to be detailed)

1. An inventory of personal data: every table, column, file and log line that can hold it (the first deliverable, before any code).
2. Anonymising a person: the actor record, tokens and sessions, the structured mentions, the person's files and planner data; audited (without personal data in the audit entry).
3. Crypto-shredding or moving personal data out of the chained payloads (to be decided).
4. Export as a background job, with a download link that expires.
5. The PWA: the owner's "Felhasználó anonimizálása" with a clear confirmation, a person's "Adataim letöltése".
6. Tests: after anonymising, no trace of the old name, handle or e-mail anywhere the API returns (search every response of a full e2e scenario); threads still render; the audit chain still verifies.
